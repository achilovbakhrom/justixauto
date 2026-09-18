package inbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
)

const quarantineArtifact = "1c1e7d17efa8ff84f2469d6d55b3b6c810143e5260aabf5f82d748797f2296a3"

var (
	ErrInvalidQuarantine        = errors.New("inbox: invalid quarantine evidence")
	ErrQuarantineConflict       = errors.New("inbox: quarantine request conflicts with retained evidence")
	ErrQuarantineLease          = errors.New("inbox: custody job lease is absent, expired, or changed")
	ErrQuarantineAcknowledgment = errors.New("inbox: quarantine committed but acknowledgment failed")
)

var quarantineAggregateType = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// QuarantineStage identifies where the rejected bytes were observed. It is
// evidence, not a statement that an event or business effect was accepted.
type QuarantineStage string

const (
	QuarantineIntake         QuarantineStage = "intake"
	QuarantineDirectHandler  QuarantineStage = "direct-handler"
	QuarantineCustodyHandler QuarantineStage = "custody-handler"
)

// QuarantineStream is trusted routing context. It must not be reconstructed
// from malformed bytes. All three fields are either present together or absent.
type QuarantineStream struct {
	SourceOwner   events.Owner
	AggregateType string
	AggregateID   string
}

// CaptureInput contains stable request identities and safe evidence metadata.
// The caller creates every UUID before persistence and reuses it after an
// unknown commit. RequiredFences are owner-declared generation/guard fences;
// the receiver fence is derived from Stream and added by this adapter.
type CaptureInput struct {
	EvidenceID, RequestID       string
	HoldActionID, HoldRequestID string
	Stage                       QuarantineStage
	QueueRef, ConsumerName      string
	Stream                      *QuarantineStream
	JobEventID, LeaseOwner      string
	ReasonCode                  string
	ActorRef, AuthorityRef      string
	ScopeRef, PurposeRef        string
	RepairRef, ManifestRef      string
	ManifestHash                [32]byte
	RequiredFences              []eventstore.FenceRequest
}

// SealRequest is passed to an owner security adapter outside SQL. Raw is a
// defensive copy. The adapter must bind the exact digest and length to its
// returned ciphertext and policy/key references; no plaintext fallback exists.
type SealRequest struct {
	Raw           []byte
	RawHash       [32]byte
	RawByteLength int64
	ContextDigest [32]byte
}

// SealedEvidence is opaque protected content. The SQL layer can verify only
// ciphertext integrity and reference shape, not cryptography or retention.
type SealedEvidence struct {
	Ciphertext        []byte
	FormatRef, KeyRef string
	CapturePolicyRef  string
}

// EvidenceSecurity is an owner-supplied port. Production implementations and
// retention/key policy are separately gated; tests may inject an explicit fake.
type EvidenceSecurity interface {
	Seal(context.Context, SealRequest) (SealedEvidence, error)
	Restore(context.Context, SealedEvidence) ([]byte, error)
}

// CaptureReceipt is returned only after a known commit (or an exact retained
// request read). It permits ACK of that one intake/direct delivery; it does not
// prove schema validity, effect completion, broker durability, or authorization.
type CaptureReceipt struct {
	EvidenceID, RequestID string
	Observation           string
	Duplicate             bool
}

// Quarantine persists protected owner-local evidence. Sealing occurs before
// its transaction. The transaction is never retried automatically.
type Quarantine struct {
	owner    events.Owner
	security EvidenceSecurity
	runner   *eventstore.Transactions[*gorm.DB]
}

func NewQuarantine(db *gorm.DB, owner events.Owner, security EvidenceSecurity) (*Quarantine, error) {
	if !owner.Valid() || security == nil {
		return nil, ErrInvalidQuarantine
	}
	runner, err := eventstore.NewTransactions(db, func(tx *gorm.DB) (*gorm.DB, error) { return tx, nil })
	if err != nil {
		return nil, err
	}
	return &Quarantine{owner: owner, security: security, runner: runner}, nil
}

// Capture seals first, then commits immutable evidence and its root hold. A
// custody capture also fences and locks the exact job and atomically installs
// only its quarantine reference. It never completes a job or advances inbox or
// checkpoint state. Unknown commits return no receipt.
func (q *Quarantine) Capture(ctx context.Context, in CaptureInput, raw []byte) (CaptureReceipt, error) {
	if q == nil || q.runner == nil || q.security == nil || !validCapture(in) {
		return CaptureReceipt{}, ErrInvalidQuarantine
	}
	body := bytes.Clone(raw)
	if int64(len(body)) < 0 { // protects conversion if Go ever permits larger slices.
		return CaptureReceipt{}, ErrInvalidQuarantine
	}
	rawHash := sha256.Sum256(body)
	baseContext := captureContextDigest(q.owner, in, "")
	sealed, err := q.security.Seal(ctx, SealRequest{Raw: body, RawHash: rawHash, RawByteLength: int64(len(body)), ContextDigest: baseContext})
	clear(body)
	if err != nil {
		return CaptureReceipt{}, err
	}
	sealed.Ciphertext = bytes.Clone(sealed.Ciphertext)
	if !validSealed(sealed) {
		return CaptureReceipt{}, ErrInvalidQuarantine
	}
	var receipt CaptureReceipt
	err = q.runner.Run(ctx, func(tx *gorm.DB) error {
		var err error
		receipt, err = q.persist(ctx, tx, in, rawHash, int64(len(raw)), sealed)
		return err
	})
	if err != nil {
		return CaptureReceipt{}, err
	}
	return receipt, nil
}

// CaptureAndAcknowledge is the only ACK-bearing quarantine operation. It is
// valid for intake/direct poison deliveries, runs ACK after the durable receipt,
// and never retries ACK. Custody-handler failures already have durable custody
// and are handled through their job reference instead.
func (q *Quarantine) CaptureAndAcknowledge(ctx context.Context, in CaptureInput, raw []byte, ack func() error) (CaptureReceipt, error) {
	if ack == nil || in.Stage == QuarantineCustodyHandler {
		return CaptureReceipt{}, ErrInvalidQuarantine
	}
	r, err := q.Capture(ctx, in, raw)
	if err != nil {
		return CaptureReceipt{}, err
	}
	if err := ack(); err != nil {
		return r, errors.Join(ErrQuarantineAcknowledgment, err)
	}
	return r, nil
}

func validCapture(in CaptureInput) bool {
	for _, id := range []string{in.EvidenceID, in.RequestID, in.HoldActionID, in.HoldRequestID} {
		if !canonicalQuarantineID(id) {
			return false
		}
	}
	for _, ref := range []string{in.ActorRef, in.AuthorityRef, in.ScopeRef, in.PurposeRef, in.RepairRef, in.ManifestRef} {
		if !safeRef(ref) {
			return false
		}
	}
	if in.ManifestHash == ([32]byte{}) || !safeCode(in.ReasonCode) {
		return false
	}
	if in.Stream != nil && (!in.Stream.SourceOwner.Valid() || !quarantineAggregateType.MatchString(in.Stream.AggregateType) || !canonicalQuarantineID(in.Stream.AggregateID)) {
		return false
	}
	switch in.Stage {
	case QuarantineIntake:
		return safeRef(in.QueueRef) && in.JobEventID == "" && in.LeaseOwner == "" && len(in.RequiredFences) == 0 && (in.Stream == nil || safeRef(in.ConsumerName))
	case QuarantineDirectHandler:
		return safeRef(in.ConsumerName) && in.JobEventID == "" && in.LeaseOwner == "" && len(in.RequiredFences) == 0
	case QuarantineCustodyHandler:
		return in.QueueRef == "" && safeRef(in.ConsumerName) && in.Stream != nil && canonicalQuarantineID(in.JobEventID) && canonicalQuarantineID(in.LeaseOwner)
	default:
		return false
	}
}

func validSealed(s SealedEvidence) bool {
	return len(s.Ciphertext) > 0 && safeRef(s.FormatRef) && safeRef(s.KeyRef) && safeRef(s.CapturePolicyRef)
}

func canonicalQuarantineID(s string) bool {
	id, err := uuid.Parse(s)
	return err == nil && id != uuid.Nil && id.String() == s
}

func safeRef(s string) bool {
	return s != "" && strings.TrimSpace(s) == s && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}

func safeCode(s string) bool {
	if len(s) < 1 || len(s) > 64 || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

func captureContextDigest(owner events.Owner, in CaptureInput, observation string) [32]byte {
	h := sha256.New()
	h.Write([]byte("justixauto:quarantine-context:v1"))
	parts := []string{string(owner), string(in.Stage), in.QueueRef, in.ConsumerName, in.JobEventID, observation, in.ReasonCode}
	if in.Stream != nil {
		parts = append(parts, string(in.Stream.SourceOwner), in.Stream.AggregateType, in.Stream.AggregateID)
	}
	var n [8]byte
	for _, p := range parts {
		binary.BigEndian.PutUint64(n[:], uint64(len(p)))
		h.Write(n[:])
		h.Write([]byte(p))
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

type quarantineEvidenceRow struct {
	EvidenceID, CaptureRequestID, Stage, OwnerService string
	QueueRef, ConsumerName, SourceOwner               *string
	AggregateType, AggregateID, JobEventID            *string
	JobObservation                                    *string
	RawSha256, SealedCiphertext, CiphertextSha256     []byte
	RawByteLength                                     int64
	SealFormatRef, SealKeyRef, CapturePolicyRef       string
	ReasonCode                                        string
	CaptureContextDigest                              []byte
}

type quarantineActionRow struct {
	ActionID, EvidenceID, RequestID string
	PriorActionID                   *string
	Action                          string
	EvidenceFormatVersion           int
	ActorRef, AuthorityRef          string
	ScopeRef, PurposeRef            string
	RepairRef, IntendedPath         string
	IntendedConsumer                *string
	ManifestRef                     string
	ManifestSha256                  []byte
	OutcomeCode                     string
}

func nullableQuarantine(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (q *Quarantine) persist(ctx context.Context, tx *gorm.DB, in CaptureInput, rawHash [32]byte, rawLen int64, sealed SealedEvidence) (CaptureReceipt, error) {
	if err := requireQuarantineReady(ctx, tx, q.owner); err != nil {
		return CaptureReceipt{}, err
	}
	// Stable request serialization makes concurrent redeliveries reconcile the
	// retained row instead of surfacing a unique-key race. This is mechanical
	// idempotency only; it does not authorize the body or its trusted context.
	for _, key := range []string{"justixauto:quarantine-evidence:" + in.EvidenceID, "justixauto:quarantine-request:" + in.RequestID} {
		if err := tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", key).Error; err != nil {
			return CaptureReceipt{}, err
		}
	}
	// Resolve a known committed request before requiring a still-live custody
	// lease. Its original observation and sealed bytes remain authoritative even
	// if the job later completed or was released by a separate audited action.
	wantStatic := q.evidenceRow(in, rawHash, rawLen, sealed, "")
	var old quarantineEvidenceRow
	find := tx.WithContext(ctx).Table("eventstore.quarantine_evidence").
		Where("capture_request_id=?::uuid OR evidence_id=?::uuid", in.RequestID, in.EvidenceID).Take(&old)
	if find.Error == nil {
		if !sameCapture(old, wantStatic) {
			return CaptureReceipt{}, ErrQuarantineConflict
		}
		wantAction := holdRow(in)
		var oldAction quarantineActionRow
		read := tx.WithContext(ctx).Table("eventstore.quarantine_actions").
			Where("action_id=?::uuid OR request_id=?::uuid", in.HoldActionID, in.HoldRequestID).Take(&oldAction)
		if read.Error != nil || !sameHold(oldAction, wantAction) {
			if read.Error != nil && !errors.Is(read.Error, gorm.ErrRecordNotFound) {
				return CaptureReceipt{}, read.Error
			}
			return CaptureReceipt{}, ErrQuarantineConflict
		}
		observation := ""
		if old.JobObservation != nil {
			observation = *old.JobObservation
		}
		return CaptureReceipt{EvidenceID: in.EvidenceID, RequestID: in.RequestID, Observation: observation, Duplicate: true}, nil
	}
	if !errors.Is(find.Error, gorm.ErrRecordNotFound) {
		return CaptureReceipt{}, find.Error
	}
	observation := ""
	if in.Stage == QuarantineCustodyHandler {
		receiver, err := eventstore.ReceiverFence(q.owner, in.Stream.SourceOwner, in.Stream.AggregateType, in.Stream.AggregateID)
		if err != nil {
			return CaptureReceipt{}, err
		}
		requests := append([]eventstore.FenceRequest{receiver}, in.RequiredFences...)
		if _, err := eventstore.PrepareFences(ctx, tx, q.owner, eventstore.SharedFence, requests...); err != nil {
			return CaptureReceipt{}, err
		}
		var job struct {
			CompletedAt   *time.Time
			LeaseOwner    *string
			LeaseUntil    *time.Time
			QuarantineRef *string
		}
		read := tx.WithContext(ctx).Raw(`SELECT completed_at,lease_owner,lease_until,quarantine_ref
			FROM eventstore.dispatch_jobs WHERE consumer_name=? AND event_id=?::uuid FOR UPDATE`, in.ConsumerName, in.JobEventID).Scan(&job)
		if read.Error != nil {
			return CaptureReceipt{}, read.Error
		}
		if read.RowsAffected != 1 {
			return CaptureReceipt{}, ErrInvalidQuarantine
		}
		if job.CompletedAt != nil {
			observation = "completed"
		} else {
			observation = "unfinished"
			var live bool
			if err := tx.WithContext(ctx).Raw(`SELECT lease_owner=?::uuid AND lease_until>=clock_timestamp()
				FROM eventstore.dispatch_jobs WHERE consumer_name=? AND event_id=?::uuid`, in.LeaseOwner, in.ConsumerName, in.JobEventID).Scan(&live).Error; err != nil {
				return CaptureReceipt{}, err
			}
			if !live {
				return CaptureReceipt{}, ErrQuarantineLease
			}
			want := "quarantine:" + in.EvidenceID
			if job.QuarantineRef != nil && *job.QuarantineRef != want {
				return CaptureReceipt{}, ErrQuarantineConflict
			}
		}
	}

	wantEvidence := q.evidenceRow(in, rawHash, rawLen, sealed, observation)
	if err := tx.WithContext(ctx).Table("eventstore.quarantine_evidence").Create(&wantEvidence).Error; err != nil {
		return CaptureReceipt{}, err
	}

	wantAction := holdRow(in)
	var oldAction quarantineActionRow
	find = tx.WithContext(ctx).Table("eventstore.quarantine_actions").
		Where("action_id=?::uuid OR request_id=?::uuid", in.HoldActionID, in.HoldRequestID).Take(&oldAction)
	if find.Error == nil {
		if !sameHold(oldAction, wantAction) {
			return CaptureReceipt{}, ErrQuarantineConflict
		}
	} else if !errors.Is(find.Error, gorm.ErrRecordNotFound) {
		return CaptureReceipt{}, find.Error
	} else if err := tx.WithContext(ctx).Table("eventstore.quarantine_actions").Create(&wantAction).Error; err != nil {
		return CaptureReceipt{}, err
	}

	if in.Stage == QuarantineCustodyHandler && observation == "unfinished" {
		ref := "quarantine:" + in.EvidenceID
		update := tx.WithContext(ctx).Exec(`UPDATE eventstore.dispatch_jobs SET quarantine_ref=?
			WHERE consumer_name=? AND event_id=?::uuid AND completed_at IS NULL
			AND lease_owner=?::uuid AND lease_until>=clock_timestamp()
			AND (quarantine_ref IS NULL OR quarantine_ref=?)`, ref, in.ConsumerName, in.JobEventID, in.LeaseOwner, ref)
		if update.Error != nil {
			return CaptureReceipt{}, update.Error
		}
		if update.RowsAffected != 1 {
			return CaptureReceipt{}, ErrQuarantineLease
		}
	}
	return CaptureReceipt{EvidenceID: in.EvidenceID, RequestID: in.RequestID, Observation: observation}, nil
}

func (q *Quarantine) evidenceRow(in CaptureInput, rawHash [32]byte, rawLen int64, sealed SealedEvidence, observation string) quarantineEvidenceRow {
	contextDigest := captureContextDigest(q.owner, in, "")
	r := quarantineEvidenceRow{EvidenceID: in.EvidenceID, CaptureRequestID: in.RequestID, Stage: string(in.Stage), OwnerService: string(q.owner),
		QueueRef: nullableQuarantine(in.QueueRef), ConsumerName: nullableQuarantine(in.ConsumerName), RawSha256: bytes.Clone(rawHash[:]), RawByteLength: rawLen,
		SealedCiphertext: bytes.Clone(sealed.Ciphertext), SealFormatRef: sealed.FormatRef, SealKeyRef: sealed.KeyRef, CapturePolicyRef: sealed.CapturePolicyRef,
		ReasonCode: in.ReasonCode, CaptureContextDigest: bytes.Clone(contextDigest[:])}
	cipherHash := sha256.Sum256(sealed.Ciphertext)
	r.CiphertextSha256 = bytes.Clone(cipherHash[:])
	if in.Stream != nil {
		source, aggregateType, aggregateID := string(in.Stream.SourceOwner), in.Stream.AggregateType, in.Stream.AggregateID
		r.SourceOwner, r.AggregateType, r.AggregateID = &source, &aggregateType, &aggregateID
	}
	if in.JobEventID != "" {
		r.JobEventID, r.JobObservation = nullableQuarantine(in.JobEventID), nullableQuarantine(observation)
	}
	return r
}

func holdRow(in CaptureInput) quarantineActionRow {
	return quarantineActionRow{ActionID: in.HoldActionID, EvidenceID: in.EvidenceID, RequestID: in.HoldRequestID,
		Action: "hold", EvidenceFormatVersion: 1, ActorRef: in.ActorRef, AuthorityRef: in.AuthorityRef, ScopeRef: in.ScopeRef,
		PurposeRef: in.PurposeRef, RepairRef: in.RepairRef, IntendedPath: string(in.Stage), IntendedConsumer: nullableQuarantine(in.ConsumerName),
		ManifestRef: in.ManifestRef, ManifestSha256: bytes.Clone(in.ManifestHash[:]), OutcomeCode: "held"}
}

func sameCapture(a, b quarantineEvidenceRow) bool {
	return a.EvidenceID == b.EvidenceID && a.CaptureRequestID == b.CaptureRequestID && a.Stage == b.Stage && a.OwnerService == b.OwnerService &&
		equalStringPtr(a.QueueRef, b.QueueRef) && equalStringPtr(a.ConsumerName, b.ConsumerName) && equalStringPtr(a.SourceOwner, b.SourceOwner) &&
		equalStringPtr(a.AggregateType, b.AggregateType) && equalStringPtr(a.AggregateID, b.AggregateID) && equalStringPtr(a.JobEventID, b.JobEventID) &&
		bytes.Equal(a.RawSha256, b.RawSha256) && a.RawByteLength == b.RawByteLength &&
		a.SealFormatRef == b.SealFormatRef && a.SealKeyRef == b.SealKeyRef && a.CapturePolicyRef == b.CapturePolicyRef && a.ReasonCode == b.ReasonCode &&
		bytes.Equal(a.CaptureContextDigest, b.CaptureContextDigest)
}

func sameHold(a, b quarantineActionRow) bool {
	return a.ActionID == b.ActionID && a.EvidenceID == b.EvidenceID && a.RequestID == b.RequestID && a.PriorActionID == nil &&
		a.Action == b.Action && a.EvidenceFormatVersion == b.EvidenceFormatVersion && a.ActorRef == b.ActorRef && a.AuthorityRef == b.AuthorityRef &&
		a.ScopeRef == b.ScopeRef && a.PurposeRef == b.PurposeRef && a.RepairRef == b.RepairRef && a.IntendedPath == b.IntendedPath &&
		equalStringPtr(a.IntendedConsumer, b.IntendedConsumer) && a.ManifestRef == b.ManifestRef && bytes.Equal(a.ManifestSha256, b.ManifestSha256) && a.OutcomeCode == b.OutcomeCode
}

func equalStringPtr(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func requireQuarantineReady(ctx context.Context, tx *gorm.DB, owner events.Owner) error {
	if err := requireTransaction(tx); err != nil {
		return err
	}
	var ready bool
	err := tx.WithContext(ctx).Raw(`SELECT count(*)=1 FROM eventstore.quarantine_compatibility
		WHERE singleton AND owner_service=? AND runtime_role=current_user AND base_schema_version=2
		AND migration_revision=5 AND feature_format_version=1
		AND encode(quarantine_migration_sha256,'hex')=?`, owner, quarantineArtifact).Scan(&ready).Error
	if err != nil {
		return err
	}
	if !ready {
		return ErrInvalidQuarantine
	}
	return nil
}
