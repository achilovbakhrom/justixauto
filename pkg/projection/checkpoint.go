package projection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"regexp"

	"gorm.io/gorm"
	"justixauto/pkg/eventstore"
)

const checkpointArtifact = "41536429fb95d4b60a11866843a2ccbd6a4cf723cd919884933b6bc1d8202c4c"

// Checkpoints binds only typed owner ports to the actual caller transaction.
// It never begins, commits, retries, fetches history or acknowledges a delivery.
// Every returned error must abort the outer unit of work.
type Checkpoints[U any] struct {
	tx       *gorm.DB
	ports    U
	fences   eventstore.PreparedFences
	contract Contract
	xid      string
	identity *checkpointsIdentity
}
type checkpointsIdentity struct{ value byte }

func NewCheckpoints[U any](ctx context.Context, tx *gorm.DB, fences eventstore.PreparedFences, c Contract, bind func(*gorm.DB) (U, error)) (*Checkpoints[U], error) {
	if c.value == nil || bind == nil {
		return nil, ErrInvalid
	}
	a := &Checkpoints[U]{tx: tx, fences: fences, contract: c, identity: &checkpointsIdentity{1}}
	if err := a.require(ctx, eventstore.SharedFence); err != nil {
		return nil, err
	}
	var ready bool
	err := tx.WithContext(ctx).Raw(`SELECT count(*)=1 FROM eventstore.projection_checkpoint_compatibility
 WHERE singleton AND owner_service=? AND runtime_role=current_user AND base_schema_version=2
 AND migration_revision=4 AND feature_format_version=1
 AND encode(checkpoint_migration_sha256,'hex')=?`, c.value.LocalOwner, checkpointArtifact).Scan(&ready).Error
	if err != nil {
		return nil, err
	}
	if !ready {
		return nil, ErrInvalid
	}
	if err = tx.WithContext(ctx).Raw("SELECT pg_current_xact_id()::text").Scan(&a.xid).Error; err != nil {
		return nil, err
	}
	ports, err := bind(tx)
	if err != nil {
		return nil, err
	}
	a.ports = ports
	return a, nil
}
func (a *Checkpoints[U]) require(ctx context.Context, mode eventstore.FenceMode) error {
	if a == nil || a.contract.value == nil {
		return ErrInvalid
	}
	c := a.contract.value
	r, err := eventstore.ReceiverFence(c.LocalOwner, c.SourceOwner, c.AggregateType, c.AggregateID)
	if err != nil {
		return err
	}
	requests := append([]eventstore.FenceRequest{r}, c.RequiredFences...)
	return a.fences.Require(ctx, a.tx, c.LocalOwner, mode, requests...)
}
func (a *Checkpoints[U]) key() []any {
	c := a.contract.value
	return []any{c.Consumer, c.SourceOwner, c.AggregateType, c.AggregateID}
}

const keyWhere = "consumer_name=? AND source_owner=? AND aggregate_type=? AND aggregate_id=?"

type bootstrapRow struct {
	BootstrapID, ConsumerName, SourceOwner, AggregateType, AggregateID, ConsumerKind, Generation string
	AdmissionID                                                                                  *string
	ContractID                                                                                   string
	ContractVersion                                                                              uint32
	ContractDigest                                                                               []byte
	AuthorityRef, ScopeRef, PurposeRef, SourceCheckpointRef                                      string
	StartAfter                                                                                   int64
	SnapshotManifestRef                                                                          *string
	SnapshotManifestHash                                                                         []byte
	NoSnapshotContractRef, NewEmptyProofRef                                                      *string
	InstallationRequestID                                                                        string
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func bootstrapValue(c Contract, in BootstrapInput) bootstrapRow {
	v := c.value
	r := bootstrapRow{BootstrapID: in.BootstrapID, ConsumerName: v.Consumer, SourceOwner: string(v.SourceOwner), AggregateType: v.AggregateType, AggregateID: v.AggregateID, ConsumerKind: v.Kind, Generation: v.Generation, AdmissionID: nullable(in.AdmissionID), ContractID: v.ContractID, ContractVersion: v.ContractVersion, ContractDigest: bytes.Clone(v.ContractDigest[:]), AuthorityRef: in.AuthorityRef, ScopeRef: in.ScopeRef, PurposeRef: in.PurposeRef, SourceCheckpointRef: in.SourceCheckpointRef, StartAfter: in.StartAfter, SnapshotManifestRef: nullable(in.SnapshotManifestRef), NoSnapshotContractRef: nullable(in.NoSnapshotContractRef), NewEmptyProofRef: nullable(in.NewEmptyProofRef), InstallationRequestID: in.RequestID}
	if in.SnapshotManifestRef != "" {
		r.SnapshotManifestHash = bytes.Clone(in.SnapshotManifestHash[:])
	}
	return r
}
func exactRow(a, b bootstrapRow) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
func (a *Checkpoints[U]) bootstrap(ctx context.Context) (bootstrapRow, error) {
	var b bootstrapRow
	r := a.tx.WithContext(ctx).Table("eventstore.consumer_bootstraps").Where(keyWhere, a.key()...).Take(&b)
	if errors.Is(r.Error, gorm.ErrRecordNotFound) {
		return b, ErrReconciliationHold
	}
	if r.Error != nil {
		return b, r.Error
	}
	c := a.contract.value
	if b.ConsumerKind != c.Kind || b.Generation != c.Generation || b.ContractID != c.ContractID || b.ContractVersion != c.ContractVersion || !bytes.Equal(b.ContractDigest, c.ContractDigest[:]) {
		return b, ErrReconciliationHold
	}
	return b, nil
}

type checkpointRow struct {
	BootstrapID        string
	Position, Revision int64
	LastEventID        *string
	LastEventHash      []byte
}

func (a *Checkpoints[U]) checkpoint(ctx context.Context) (checkpointRow, error) {
	var r checkpointRow
	result := a.tx.WithContext(ctx).Raw("SELECT bootstrap_id,position,revision,last_event_id,last_event_hash FROM eventstore.consumer_checkpoints WHERE "+keyWhere+" FOR UPDATE", a.key()...).Scan(&r)
	if result.Error != nil {
		return r, result.Error
	}
	if result.RowsAffected != 1 {
		return r, ErrReconciliationHold
	}
	return r, nil
}

// InstallBootstrap revalidates current source/admission/scope authority under
// the exclusive catalog + stream fences. install maps the already verified
// snapshot to local typed read rows and must not perform network I/O. T-920
// performs admission/enrollment/backlog work in this SAME outer transaction.
// false means exact retained request replay: the snapshot is not applied twice.
// Neither true nor false is a committed receipt; reconcile after unknown commit.
func (a *Checkpoints[U]) InstallBootstrap(ctx context.Context, b VerifiedBootstrap, revalidate func(context.Context, U, VerifiedBootstrap) error, install func(context.Context, U) error) (bool, error) {
	if !b.valid || a == nil || !a.contract.same(b.contract) || revalidate == nil || install == nil {
		return false, ErrInvalid
	}
	if err := a.require(ctx, eventstore.ExclusiveFence); err != nil {
		return false, err
	}
	if err := revalidate(ctx, a.ports, b); err != nil {
		return false, err
	}
	want := bootstrapValue(a.contract, b.input)
	var old bootstrapRow
	res := a.tx.WithContext(ctx).Table("eventstore.consumer_bootstraps").Where("installation_request_id=? OR ("+keyWhere+")", append([]any{b.input.RequestID}, a.key()...)...).Take(&old)
	if res.Error == nil {
		if !exactRow(old, want) {
			return false, ErrConflict
		}
		cp, err := a.checkpoint(ctx)
		if err != nil {
			return false, err
		}
		if cp.BootstrapID != old.BootstrapID || cp.Position < old.StartAfter || cp.Position-old.StartAfter != cp.Revision {
			return false, ErrReconciliationHold
		}
		return false, nil
	}
	if !errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return false, res.Error
	}
	// Consumer meaning is immutable across its complete stream universe. The
	// exclusive catalog fence prevents two first streams racing this check.
	var mismatches int64
	c := a.contract.value
	if err := a.tx.WithContext(ctx).Table("eventstore.consumer_bootstraps").Where("consumer_name=? AND (consumer_kind<>? OR generation<>? OR contract_id<>? OR contract_version<>? OR contract_digest<>?)", c.Consumer, c.Kind, c.Generation, c.ContractID, c.ContractVersion, c.ContractDigest[:]).Count(&mismatches).Error; err != nil {
		return false, err
	}
	if mismatches != 0 {
		return false, ErrConflict
	}
	if err := a.tx.WithContext(ctx).Table("eventstore.consumer_bootstraps").Create(&want).Error; err != nil {
		return false, err
	}
	if err := install(ctx, a.ports); err != nil {
		return false, err
	}
	res = a.tx.WithContext(ctx).Exec(`INSERT INTO eventstore.consumer_checkpoints(consumer_name,source_owner,aggregate_type,aggregate_id,bootstrap_id,consumer_kind,generation,position,revision)
 SELECT consumer_name,source_owner,aggregate_type,aggregate_id,bootstrap_id,consumer_kind,generation,start_after,0 FROM eventstore.consumer_bootstraps WHERE bootstrap_id=?`, b.input.BootstrapID)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected != 1 {
		return false, ErrConflict
	}
	return true, nil
}

// NextCandidate is sealed to this adapter/transaction and exact pre-effect
// checkpoint. It cannot authorize another consumer, stream, hash or position.
type NextCandidate struct {
	issuer           *checkpointsIdentity
	position         ValidatedPosition
	bootstrap        string
	before, revision int64
}

func (a *Checkpoints[U]) CheckNext(ctx context.Context, p ValidatedPosition, revalidate func(context.Context, U, ValidatedPosition) error) (NextCandidate, error) {
	if a == nil || !a.contract.same(p.contract) || revalidate == nil {
		return NextCandidate{}, ErrInvalid
	}
	if err := a.require(ctx, eventstore.SharedFence); err != nil {
		return NextCandidate{}, err
	}
	if err := revalidate(ctx, a.ports, p); err != nil {
		return NextCandidate{}, err
	}
	b, err := a.bootstrap(ctx)
	if err != nil {
		return NextCandidate{}, err
	}
	cp, err := a.checkpoint(ctx)
	if err != nil {
		return NextCandidate{}, err
	}
	if cp.BootstrapID != b.BootstrapID || cp.Position < b.StartAfter || cp.Position-b.StartAfter != cp.Revision {
		return NextCandidate{}, ErrReconciliationHold
	}
	seq := p.envelope.IntegrationSequence().Int64()
	if seq <= cp.Position || cp.Position == math.MaxInt64 {
		return NextCandidate{}, ErrReconciliationHold
	}
	if seq > cp.Position+1 {
		return NextCandidate{}, &Gap{p, b.BootstrapID, cp.Position + 1, a.xid}
	}
	return NextCandidate{a.identity, p, b.BootstrapID, cp.Position, cp.Revision}, nil
}

// Advance expects T-013/T-922 to have inserted the matching inbox first. The
// owner effect and +1 checkpoint update remain uncommitted. Known irrelevant
// events skip apply but still advance; unsupported schemas never reach here.
func (a *Checkpoints[U]) Advance(ctx context.Context, n NextCandidate, apply func(context.Context, U, ValidatedPosition) error) error {
	if a == nil || n.issuer == nil || n.issuer != a.identity || apply == nil {
		return ErrInvalid
	}
	if err := a.require(ctx, eventstore.SharedFence); err != nil {
		return err
	}
	cp, err := a.checkpoint(ctx)
	if err != nil {
		return err
	}
	if cp.BootstrapID != n.bootstrap || cp.Position != n.before || cp.Revision != n.revision {
		return ErrConflict
	}
	var count int64
	if err := a.tx.WithContext(ctx).Table("eventstore.inbox").Where("consumer_name=? AND event_id=? AND envelope_hash=?", a.contract.value.Consumer, n.position.envelope.EventID(), n.position.hash[:]).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrReconciliationHold
	}
	if !n.position.irrelevant {
		if err := apply(ctx, a.ports, n.position); err != nil {
			return err
		}
	}
	res := a.tx.WithContext(ctx).Exec(`UPDATE eventstore.consumer_checkpoints SET position=position+1,revision=revision+1,last_event_id=?,last_event_hash=?,updated_at=clock_timestamp() WHERE `+keyWhere+` AND bootstrap_id=? AND position=? AND revision=?`, append([]any{n.position.envelope.EventID(), n.position.hash[:]}, append(a.key(), n.bootstrap, n.before, n.revision)...)...)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrConflict
	}
	return nil
}

// DuplicateCoverage asks an owner port to verify retained original event/stream
// coverage inside the SAME transaction. A higher checkpoint integer alone is
// not evidence. There is no default verifier or remote history call here.
type DuplicateCoverage struct {
	Position            ValidatedPosition
	BootstrapID         string
	StartAfter, Through int64
	LastEventID         string
	LastEventHash       [32]byte
}

func (a *Checkpoints[U]) VerifyDuplicate(ctx context.Context, p ValidatedPosition, revalidate func(context.Context, U, ValidatedPosition) error, coverage func(context.Context, U, DuplicateCoverage) error) error {
	if a == nil || !a.contract.same(p.contract) || revalidate == nil {
		return ErrInvalid
	}
	if err := a.require(ctx, eventstore.SharedFence); err != nil {
		return err
	}
	if err := revalidate(ctx, a.ports, p); err != nil {
		return err
	}
	b, err := a.bootstrap(ctx)
	if err != nil {
		return err
	}
	cp, err := a.checkpoint(ctx)
	if err != nil {
		return err
	}
	seq := p.envelope.IntegrationSequence().Int64()
	if cp.BootstrapID != b.BootstrapID || cp.Position < b.StartAfter || cp.Position-b.StartAfter != cp.Revision || seq <= b.StartAfter || seq > cp.Position || cp.LastEventID == nil {
		return ErrReconciliationHold
	}
	var count int64
	if err := a.tx.WithContext(ctx).Table("eventstore.inbox").Where("consumer_name=? AND event_id=? AND envelope_hash=?", a.contract.value.Consumer, p.envelope.EventID(), p.hash[:]).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrReconciliationHold
	}
	if seq == cp.Position {
		if *cp.LastEventID != p.envelope.EventID() || !bytes.Equal(cp.LastEventHash, p.hash[:]) {
			return ErrReconciliationHold
		}
		return nil
	}
	if coverage == nil {
		return ErrReconciliationHold
	}
	var hash [32]byte
	copy(hash[:], cp.LastEventHash)
	return coverage(ctx, a.ports, DuplicateCoverage{p, b.BootstrapID, b.StartAfter, cp.Position, *cp.LastEventID, hash})
}

type GapEvidence struct{ GapID, RequestID, AttemptID, AttemptRequestID, AdmissionRef, AuthorityRef string }
type gapRow struct {
	GapID, ConsumerName, SourceOwner, AggregateType, AggregateID, BootstrapID, BlockedEventID string
	BlockedEventHash                                                                          []byte
	ExpectedPosition, ObservedPosition                                                        int64
	AdmissionRef, AuthorityRef, RequestID                                                     string
}
type attemptRow struct {
	AttemptID, GapID, RequestID, Action string
	PriorAttemptID                      *string
	EvidenceFormatVersion               int
	IntervalFrom, IntervalThrough       int64
	RecoveryManifestRef                 *string
	RecoveryManifestHash                []byte
	HoldReasonCode                      *string
	AuthorityRef                        string
	ResultingPosition                   int64
}

// GapRecord is a persistence candidate. Discard it on failed/unknown commit;
// ReadRecovery authoritatively reconciles exact IDs before external fetching.
type GapRecord struct {
	GapID, AttemptID string
	Resolved         bool
}

// GapStatus lets a restarted worker discover the retained request and exact
// chain tip from the original validated blocked envelope. IDs are never guessed
// from delivery count, and finding a gap does not authorize another recovery.
type GapStatus struct {
	GapID, RequestID, AttemptID, AttemptRequestID, Action, AuthorityRef string
	Expected, Observed, Position                                        int64
}

func (a *Checkpoints[U]) FindGap(ctx context.Context, p ValidatedPosition, revalidate func(context.Context, U, ValidatedPosition) error) (GapStatus, error) {
	if a == nil || !a.contract.same(p.contract) || revalidate == nil {
		return GapStatus{}, ErrInvalid
	}
	if err := a.require(ctx, eventstore.SharedFence); err != nil {
		return GapStatus{}, err
	}
	if err := revalidate(ctx, a.ports, p); err != nil {
		return GapStatus{}, err
	}
	b, err := a.bootstrap(ctx)
	if err != nil {
		return GapStatus{}, err
	}
	cp, err := a.checkpoint(ctx)
	if err != nil {
		return GapStatus{}, err
	}
	if cp.BootstrapID != b.BootstrapID || cp.Position < b.StartAfter || cp.Position-b.StartAfter != cp.Revision {
		return GapStatus{}, ErrReconciliationHold
	}
	var g gapRow
	r := a.tx.WithContext(ctx).Table("eventstore.consumer_gaps").Where(keyWhere+" AND blocked_event_id=?", append(a.key(), p.envelope.EventID())...).Take(&g)
	if r.Error != nil {
		return GapStatus{}, r.Error
	}
	if g.BootstrapID != b.BootstrapID || g.ObservedPosition != p.envelope.IntegrationSequence().Int64() || !bytes.Equal(g.BlockedEventHash, p.hash[:]) {
		return GapStatus{}, ErrConflict
	}
	var tips []attemptRow
	r = a.tx.WithContext(ctx).Raw(`SELECT a.* FROM eventstore.consumer_gap_attempts a WHERE a.gap_id=?
 AND NOT EXISTS(SELECT FROM eventstore.consumer_gap_attempts child WHERE child.prior_attempt_id=a.attempt_id)`, g.GapID).Scan(&tips)
	if r.Error != nil {
		return GapStatus{}, r.Error
	}
	if len(tips) != 1 {
		return GapStatus{}, ErrReconciliationHold
	}
	tip := tips[0]
	return GapStatus{g.GapID, g.RequestID, tip.AttemptID, tip.RequestID, tip.Action, tip.AuthorityRef, g.ExpectedPosition, g.ObservedPosition, cp.Position}, nil
}

func (a *Checkpoints[U]) RecordGap(ctx context.Context, g *Gap, e GapEvidence, authorize func(context.Context, U, GapEvidence) error) (GapRecord, error) {
	if a == nil || g == nil || !a.contract.same(g.position.contract) || authorize == nil {
		return GapRecord{}, ErrInvalid
	}
	for _, id := range []string{e.GapID, e.RequestID, e.AttemptID, e.AttemptRequestID} {
		if !canonicalID(id) {
			return GapRecord{}, ErrInvalid
		}
	}
	if !nonempty(e.AdmissionRef) || !nonempty(e.AuthorityRef) {
		return GapRecord{}, ErrInvalid
	}
	if err := a.require(ctx, eventstore.SharedFence); err != nil {
		return GapRecord{}, err
	}
	if a.xid == g.xid {
		return GapRecord{}, ErrSeparateTransaction
	}
	if err := authorize(ctx, a.ports, e); err != nil {
		return GapRecord{}, err
	}
	b, err := a.bootstrap(ctx)
	if err != nil {
		return GapRecord{}, err
	}
	cp, err := a.checkpoint(ctx)
	if err != nil {
		return GapRecord{}, err
	}
	if cp.BootstrapID != g.bootstrap || b.BootstrapID != g.bootstrap {
		return GapRecord{}, ErrReconciliationHold
	}
	want := gapRow{e.GapID, a.contract.value.Consumer, string(a.contract.value.SourceOwner), a.contract.value.AggregateType, a.contract.value.AggregateID, g.bootstrap, g.position.envelope.EventID(), bytes.Clone(g.position.hash[:]), g.expected, g.position.envelope.IntegrationSequence().Int64(), e.AdmissionRef, e.AuthorityRef, e.RequestID}
	var old gapRow
	res := a.tx.WithContext(ctx).Table("eventstore.consumer_gaps").Where("request_id=? OR ("+keyWhere+" AND blocked_event_id=?)", append([]any{e.RequestID}, append(a.key(), want.BlockedEventID)...)...).Take(&old)
	if res.Error == nil {
		x, _ := json.Marshal(old)
		y, _ := json.Marshal(want)
		if !bytes.Equal(x, y) {
			return GapRecord{}, ErrConflict
		}
		var attempt attemptRow
		res = a.tx.WithContext(ctx).Table("eventstore.consumer_gap_attempts").Where("attempt_id=? AND request_id=? AND gap_id=? AND prior_attempt_id IS NULL", e.AttemptID, e.AttemptRequestID, e.GapID).Take(&attempt)
		if res.Error != nil {
			return GapRecord{}, ErrConflict
		}
		return GapRecord{e.GapID, e.AttemptID, attempt.Action == "resolved"}, nil
	}
	if !errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return GapRecord{}, res.Error
	}
	if err := a.tx.WithContext(ctx).Table("eventstore.consumer_gaps").Create(&want).Error; err != nil {
		return GapRecord{}, err
	}
	action := "requested"
	if cp.Position >= want.ObservedPosition-1 {
		action = "resolved"
	}
	attempt := attemptRow{AttemptID: e.AttemptID, GapID: e.GapID, RequestID: e.AttemptRequestID, Action: action, EvidenceFormatVersion: 1, IntervalFrom: want.ExpectedPosition, IntervalThrough: want.ObservedPosition - 1, AuthorityRef: e.AuthorityRef, ResultingPosition: cp.Position}
	if action == "requested" {
		attempt.IntervalFrom = cp.Position + 1
	}
	if err := a.tx.WithContext(ctx).Table("eventstore.consumer_gap_attempts").Create(&attempt).Error; err != nil {
		return GapRecord{}, err
	}
	return GapRecord{e.GapID, e.AttemptID, action == "resolved"}, nil
}

type AttemptInput struct {
	GapID, AttemptID, RequestID, PriorAttemptID, Action, AuthorityRef, HoldReasonCode string
	Recovery                                                                          VerifiedRecovery
}

var reasonPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// AppendAttempt serializes with checkpoint mutation, compares the exact chain
// tip, and rereads progress. A stale request/hold returns a reconciliation hold
// so its caller records an explicit resolved action; no attempt advances a
// checkpoint and no changed request is silently normalized into an old receipt.
func (a *Checkpoints[U]) AppendAttempt(ctx context.Context, in AttemptInput, authorize func(context.Context, U, AttemptInput) error) (GapRecord, error) {
	if a == nil || authorize == nil || !nonempty(in.AuthorityRef) {
		return GapRecord{}, ErrInvalid
	}
	for _, id := range []string{in.GapID, in.AttemptID, in.RequestID, in.PriorAttemptID} {
		if !canonicalID(id) {
			return GapRecord{}, ErrInvalid
		}
	}
	if in.Action != "requested" && in.Action != "recovered" && in.Action != "held" && in.Action != "resumed" && in.Action != "resolved" {
		return GapRecord{}, ErrInvalid
	}
	if (in.Action == "held" && !reasonPattern.MatchString(in.HoldReasonCode)) || (in.Action != "held" && in.HoldReasonCode != "") {
		return GapRecord{}, ErrInvalid
	}
	if err := a.require(ctx, eventstore.SharedFence); err != nil {
		return GapRecord{}, err
	}
	if err := authorize(ctx, a.ports, in); err != nil {
		return GapRecord{}, err
	}
	var g gapRow
	res := a.tx.WithContext(ctx).Table("eventstore.consumer_gaps").Where("gap_id=? AND "+keyWhere, append([]any{in.GapID}, a.key()...)...).Take(&g)
	if res.Error != nil {
		return GapRecord{}, res.Error
	}
	cp, err := a.checkpoint(ctx)
	if err != nil {
		return GapRecord{}, err
	}
	if cp.BootstrapID != g.BootstrapID {
		return GapRecord{}, ErrReconciliationHold
	}
	var prior attemptRow
	res = a.tx.WithContext(ctx).Table("eventstore.consumer_gap_attempts").Where("attempt_id=? AND gap_id=?", in.PriorAttemptID, in.GapID).Take(&prior)
	if res.Error != nil {
		return GapRecord{}, res.Error
	}
	if prior.Action == "resolved" {
		return GapRecord{}, ErrConflict
	}
	action := in.Action
	want := attemptRow{AttemptID: in.AttemptID, GapID: in.GapID, RequestID: in.RequestID, Action: action, PriorAttemptID: nullable(in.PriorAttemptID), EvidenceFormatVersion: 1, IntervalFrom: g.ExpectedPosition, IntervalThrough: g.ObservedPosition - 1, AuthorityRef: in.AuthorityRef, ResultingPosition: cp.Position}
	if action == "requested" && cp.Position < g.ObservedPosition-1 {
		want.IntervalFrom = cp.Position + 1
	}
	if action == "held" {
		want.HoldReasonCode = nullable(in.HoldReasonCode)
	}
	if action == "recovered" {
		r := in.Recovery
		if !r.request.contract.same(a.contract) || r.request.gapID != in.GapID || r.request.attemptID != in.PriorAttemptID || prior.Action != "requested" || r.request.from != prior.IntervalFrom || r.request.through != prior.IntervalThrough {
			return GapRecord{}, ErrConflict
		}
		want.RecoveryManifestRef = nullable(r.batch.ManifestRef)
		want.RecoveryManifestHash = bytes.Clone(r.batch.ManifestHash[:])
		want.IntervalFrom = r.request.from
		want.IntervalThrough = r.request.through
	}
	var existing attemptRow
	res = a.tx.WithContext(ctx).Table("eventstore.consumer_gap_attempts").Where("request_id=? OR prior_attempt_id=?", in.RequestID, in.PriorAttemptID).Take(&existing)
	if res.Error == nil {
		// Reconcile immutable evidence, not today's mutable progress. A later
		// checkpoint must not make an exact request replay look conflicting.
		want.ResultingPosition = existing.ResultingPosition
		if action == "requested" {
			want.IntervalFrom = existing.IntervalFrom
		}
		x, _ := json.Marshal(existing)
		y, _ := json.Marshal(want)
		if !bytes.Equal(x, y) {
			return GapRecord{}, ErrConflict
		}
		return GapRecord{in.GapID, in.AttemptID, existing.Action == "resolved"}, nil
	}
	if !errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return GapRecord{}, res.Error
	}
	if (action == "resolved") != (cp.Position >= g.ObservedPosition-1) {
		return GapRecord{}, ErrReconciliationHold
	}
	if (action == "resumed" && prior.Action != "held") || (action == "requested" && prior.Action == "held") || (action == "held" && prior.Action == "held") {
		return GapRecord{}, ErrConflict
	}
	if err := a.tx.WithContext(ctx).Table("eventstore.consumer_gap_attempts").Create(&want).Error; err != nil {
		return GapRecord{}, err
	}
	return GapRecord{in.GapID, in.AttemptID, action == "resolved"}, nil
}

// ReadRecovery accepts a pool only and executes one authoritative committed
// read. Call after the gap/attempt transaction has ended. It verifies the exact
// requested tip and refuses held/resolved/stale evidence before external I/O.
// authorize verifies CURRENT source purpose/scope outside SQL; no source policy
// is inferred from custody or a bare evidence reference.
func ReadRecovery(ctx context.Context, db *gorm.DB, c Contract, gapID, attemptID, requestID string, authorize func(context.Context, RecoveryRequest) error) (RecoveryRequest, error) {
	if db == nil || db.Statement == nil || c.value == nil || authorize == nil || !canonicalID(gapID) || !canonicalID(attemptID) || !canonicalID(requestID) {
		return RecoveryRequest{}, ErrInvalid
	}
	if _, ok := db.Statement.ConnPool.(gorm.TxCommitter); ok {
		return RecoveryRequest{}, ErrSeparateTransaction
	}
	var r struct {
		AuthorityRef                  string
		IntervalFrom, IntervalThrough int64
	}
	v := c.value
	result := db.WithContext(ctx).Raw(`SELECT a.authority_ref,a.interval_from,a.interval_through FROM eventstore.consumer_gap_attempts a
 JOIN eventstore.consumer_gaps g USING(gap_id) JOIN eventstore.consumer_checkpoints c ON (c.consumer_name,c.source_owner,c.aggregate_type,c.aggregate_id)=(g.consumer_name,g.source_owner,g.aggregate_type,g.aggregate_id)
 JOIN eventstore.consumer_bootstraps b ON b.bootstrap_id=c.bootstrap_id
 JOIN eventstore.projection_checkpoint_compatibility m ON m.singleton AND m.owner_service=? AND m.runtime_role=current_user
 WHERE g.gap_id=? AND a.attempt_id=? AND a.request_id=? AND a.action='requested'
 AND NOT EXISTS(SELECT FROM eventstore.consumer_gap_attempts child WHERE child.prior_attempt_id=a.attempt_id)
 AND c.position=a.interval_from-1 AND c.position<a.interval_through AND c.bootstrap_id=g.bootstrap_id AND b.consumer_name=? AND b.source_owner=? AND b.aggregate_type=? AND b.aggregate_id=?
 AND b.consumer_kind=? AND b.generation=? AND b.contract_id=? AND b.contract_version=? AND b.contract_digest=?`, v.LocalOwner, gapID, attemptID, requestID, v.Consumer, v.SourceOwner, v.AggregateType, v.AggregateID, v.Kind, v.Generation, v.ContractID, v.ContractVersion, v.ContractDigest[:]).Scan(&r)
	if result.Error != nil {
		return RecoveryRequest{}, result.Error
	}
	if result.RowsAffected != 1 {
		return RecoveryRequest{}, ErrReconciliationHold
	}
	req := RecoveryRequest{c, gapID, attemptID, r.AuthorityRef, r.IntervalFrom, r.IntervalThrough}
	if err := authorize(ctx, req); err != nil {
		return RecoveryRequest{}, err
	}
	return req, nil
}
