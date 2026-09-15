package outbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
)

var (
	ErrInvalidAdmission     = errors.New("invalid source admission")
	ErrAdmissionConflict    = errors.New("conflicting source admission evidence")
	ErrAdmissionCheckpoint  = errors.New("source admission checkpoint does not match fenced high-water")
	ErrAdmissionHeld        = errors.New("source admission is held")
	ErrAdmissionSchema      = errors.New("schema is not admitted for the complete stream")
	ErrMissingRecipientPlan = errors.New("no authorized recipient or explicit empty-plan rule")
	ErrUnfencedStream       = errors.New("source stream is outside the transaction fence")
)

var admissionType = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var admissionSchema = regexp.MustCompile(`^([a-z][a-z0-9-]*)(\.[a-z][a-z0-9-]*)+\.v[1-9][0-9]*$`)

// SourceStream is concrete: wildcard/company predicates are not admissions.
// Its canonical UUID representation also matches T-009's advisory-lock key.
type SourceStream struct {
	owner                      events.Owner
	aggregateType, aggregateID string
}

func NewSourceStream(owner events.Owner, aggregateType, aggregateID string) (SourceStream, error) {
	if !owner.Valid() || !admissionType.MatchString(aggregateType) || !admissionUUID(aggregateID) {
		return SourceStream{}, ErrInvalidAdmission
	}
	return SourceStream{owner, aggregateType, aggregateID}, nil
}
func (s SourceStream) Owner() events.Owner   { return s.owner }
func (s SourceStream) AggregateType() string { return s.aggregateType }
func (s SourceStream) AggregateID() string   { return s.aggregateID }
func (s SourceStream) key() string           { return s.aggregateType + ":" + s.aggregateID }
func (s SourceStream) valid() bool {
	_, err := NewSourceStream(s.owner, s.aggregateType, s.aggregateID)
	return err == nil
}

type AdmissionContract struct {
	ID      string
	Version uint32
	Digest  [sha256.Size]byte
}

// BootstrapEvidence is supplied only after the owner authorizes the bootstrap
// protocol. Ref identifies its checkpoint/snapshot evidence, not a broadcast
// payload. EmptyStreamRef explicitly establishes a new stream when Checkpoint=0.
// This adapter verifies local history; it cannot authenticate an external proof.
type BootstrapEvidence struct {
	Checkpoint          events.Revision
	Ref, EmptyStreamRef string
}

type SourceAdmissionInput struct {
	ID                              string
	Stream                          SourceStream
	Destination                     events.Owner
	Contract                        AdmissionContract
	Schemas                         []string
	AuthorityRef, ScopeRef, Purpose string
	Bootstrap                       BootstrapEvidence
}

type SourceAdmission struct{ input SourceAdmissionInput }

func NewSourceAdmission(in SourceAdmissionInput) (SourceAdmission, error) {
	if !admissionUUID(in.ID) || !in.Stream.valid() || !in.Destination.Valid() || !validContract(in.Contract) ||
		!evidence(in.AuthorityRef) || !evidence(in.ScopeRef) || !evidence(in.Purpose) || !evidence(in.Bootstrap.Ref) ||
		(in.Bootstrap.Checkpoint.IsZero() && !evidence(in.Bootstrap.EmptyStreamRef)) {
		return SourceAdmission{}, ErrInvalidAdmission
	}
	schemas, err := checkedSchemas(in.Stream.owner, in.Schemas)
	if err != nil {
		return SourceAdmission{}, err
	}
	in.Schemas = schemas
	return SourceAdmission{in}, nil
}

// SourcePlanRule is an owner-approved concrete stream emission rule. A missing
// rule is never an empty plan. EmptyAuthorityRef must independently authorize
// emitting without active recipients. These references are supplied by the
// owner adapter after business authorization; strings themselves grant nothing.
type SourcePlanRule struct {
	stream                    SourceStream
	authority, emptyAuthority string
	schemas                   []string
}

func NewSourcePlanRule(stream SourceStream, authority string, schemas []string, emptyAuthorityRef string) (SourcePlanRule, error) {
	if !stream.valid() || !evidence(authority) || (emptyAuthorityRef != "" && !evidence(emptyAuthorityRef)) {
		return SourcePlanRule{}, ErrInvalidAdmission
	}
	checked, err := checkedSchemas(stream.owner, schemas)
	if err != nil {
		return SourcePlanRule{}, err
	}
	return SourcePlanRule{stream, authority, emptyAuthorityRef, checked}, nil
}

type AdmissionAction string

const (
	AdmissionClose  AdmissionAction = "close"
	AdmissionHold   AdmissionAction = "hold"
	AdmissionResume AdmissionAction = "resume"
)

// Changes append evidence against the current tip; they never edit an admit.
// Close drains obligations through the checked high-water. Hold pauses all
// unsent obligations of this admission, including existing leases, and blocks
// further emission while the range is active. It cannot recall in-flight bytes.
// Resume needs an explicit authority for releasing retained work; it does not
// reopen a closed range. Re-admission uses a fresh ID and bootstrap instead.
type SourceAdmissionChange struct {
	ID, PriorID               string
	Action                    AdmissionAction
	Checkpoint                events.Revision
	AuthorityRef, EvidenceRef string
	RetainedWorkAuthorityRef  string
}

type PlanRecipient struct {
	Destination events.Owner
	AdmissionID string
	Contract    AdmissionContract
}

// DeliveryPlan is a candidate for the same transaction, not a committed receipt.
// T-919 must resolve immediately while writing, never accept caller destinations
// or a cached plan. The private transaction and evidence fingerprint prevent
// accidental cross-transaction reuse; accessors return value/copy data only.
type DeliveryPlan struct {
	stream             SourceStream
	eventID, eventType string
	sequence           events.Revision
	recipients         []PlanRecipient
	authority          string
	issuer             *SourceAdmissions
	evidenceHash       [sha256.Size]byte
}

func (p DeliveryPlan) Stream() SourceStream        { return p.stream }
func (p DeliveryPlan) Sequence() events.Revision   { return p.sequence }
func (p DeliveryPlan) Recipients() []PlanRecipient { return slices.Clone(p.recipients) }
func (p DeliveryPlan) AuthorityRef() string        { return p.authority }

// SourceAdmissions binds to T-009's exact transaction. Fence must be called once
// with ALL streams this command will touch, before any Appender.Append call or
// other stream lock. Do not use concurrent goroutines inside one UnitOfWork.
// Propagate every error to Run so admission, append, guards and plans roll back.
type SourceAdmissions struct {
	tx     *gorm.DB
	owner  events.Owner
	fenced map[SourceStream]bool
}

func NewSourceAdmissions(tx *gorm.DB, owner events.Owner) (*SourceAdmissions, error) {
	if err := requireTransaction(tx); err != nil {
		return nil, err
	}
	if !owner.Valid() {
		return nil, ErrInvalidAdmission
	}
	return &SourceAdmissions{tx: tx, owner: owner}, nil
}
func (a *SourceAdmissions) Fence(ctx context.Context, streams ...SourceStream) error {
	if a == nil {
		return eventstore.ErrTransactionRequired
	}
	if err := requireTransaction(a.tx); err != nil {
		return err
	}
	if a.fenced != nil || len(streams) == 0 {
		return ErrUnfencedStream
	}
	ordered := slices.Clone(streams)
	slices.SortFunc(ordered, func(x, y SourceStream) int { return strings.Compare(x.key(), y.key()) })
	for i, s := range ordered {
		if !s.valid() || s.owner != a.owner || (i > 0 && s == ordered[i-1]) {
			return ErrInvalidAdmission
		}
	}
	var count int64
	if err := a.tx.WithContext(ctx).Raw(`SELECT count(*) FROM eventstore.messaging_mode WHERE singleton AND owner_service=? AND schema_version=2`, string(a.owner)).Scan(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrInvalidAdmission
	}
	for _, s := range ordered {
		if err := a.tx.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "justixauto:eventstore:"+string(a.owner)+":"+s.key()).Error; err != nil {
			return err
		}
	}
	a.fenced = make(map[SourceStream]bool, len(ordered))
	for _, s := range ordered {
		a.fenced[s] = true
	}
	return nil
}

func (a *SourceAdmissions) check(s SourceStream) error {
	if a == nil {
		return eventstore.ErrTransactionRequired
	}
	if err := requireTransaction(a.tx); err != nil {
		return err
	}
	if !a.fenced[s] {
		return ErrUnfencedStream
	}
	return nil
}

type sourceAdmissionRow struct {
	AdmissionID, Subject, Action                  string
	PriorAdmissionID                              sql.NullString
	ContractID                                    string
	ContractVersion                               uint32
	ContractDigest                                []byte
	SchemaAllowlist                               []byte
	AuthorityRef, ScopeRef, Purpose, BootstrapRef string
	StartAfter                                    int64
	EndInclusive                                  sql.NullInt64
}
type effectiveAdmission struct {
	root, tip sourceAdmissionRow
	end       sql.NullInt64
	held      string
	chain     []string
}

func (a *SourceAdmissions) load(ctx context.Context, s SourceStream) ([]effectiveAdmission, [sha256.Size]byte, error) {
	var rows []sourceAdmissionRow
	err := a.tx.WithContext(ctx).Raw(`SELECT admission_id,subject,action,prior_admission_id,contract_id,contract_version,
 contract_digest,schema_allowlist,authority_ref,scope_ref,purpose,bootstrap_ref,start_after,end_inclusive
 FROM eventstore.messaging_admissions WHERE namespace='source-recipient' AND source_owner=?
 AND aggregate_type=? AND aggregate_id=?::uuid ORDER BY admission_id`, string(s.owner), s.aggregateType, s.aggregateID).Scan(&rows).Error
	if err != nil {
		return nil, [sha256.Size]byte{}, err
	}
	encoded, _ := json.Marshal(rows)
	hash := sha256.Sum256(encoded)
	states, err := resolveAdmissionRows(s, rows)
	return states, hash, err
}

// Resolve every root and linear evidence chain, even inactive ones: corrupt or
// branching evidence is not hidden by selecting only the newest timestamp.
func resolveAdmissionRows(s SourceStream, rows []sourceAdmissionRow) ([]effectiveAdmission, error) {
	byID := make(map[string]sourceAdmissionRow, len(rows))
	next := make(map[string]string, len(rows))
	for _, r := range rows {
		var schemas []string
		if !admissionUUID(r.AdmissionID) || !events.Owner(r.Subject).Valid() || len(r.ContractDigest) != sha256.Size || !validContract(rowContract(r)) ||
			!evidence(r.AuthorityRef) || !evidence(r.ScopeRef) || !evidence(r.Purpose) || !evidence(r.BootstrapRef) ||
			r.StartAfter < 0 || (r.EndInclusive.Valid && r.EndInclusive.Int64 < r.StartAfter) ||
			json.Unmarshal(r.SchemaAllowlist, &schemas) != nil {
			return nil, ErrAdmissionConflict
		}
		if _, err := checkedSchemas(s.owner, schemas); err != nil {
			return nil, ErrAdmissionConflict
		}
		if _, ok := byID[r.AdmissionID]; ok {
			return nil, ErrAdmissionConflict
		}
		byID[r.AdmissionID] = r
		if r.Action == "admit" {
			if r.PriorAdmissionID.Valid {
				return nil, ErrAdmissionConflict
			}
		} else {
			if !r.PriorAdmissionID.Valid || next[r.PriorAdmissionID.String] != "" {
				return nil, ErrAdmissionConflict
			}
			next[r.PriorAdmissionID.String] = r.AdmissionID
		}
	}
	visited := make(map[string]bool, len(rows))
	states := make([]effectiveAdmission, 0)
	for _, root := range rows {
		if root.Action != "admit" {
			continue
		}
		state := effectiveAdmission{root: root, tip: root, end: root.EndInclusive, chain: []string{root.AdmissionID}}
		visited[root.AdmissionID] = true
		for id := next[root.AdmissionID]; id != ""; id = next[id] {
			r, ok := byID[id]
			if !ok || visited[id] || r.Subject != root.Subject || !sameAdmissionMetadata(root, r) {
				return nil, ErrAdmissionConflict
			}
			visited[id] = true
			switch r.Action {
			case "close":
				if state.end.Valid || !r.EndInclusive.Valid {
					return nil, ErrAdmissionConflict
				}
				state.end = r.EndInclusive
			case "hold":
				if state.held != "" || r.EndInclusive != state.end {
					return nil, ErrAdmissionConflict
				}
				state.held = r.AdmissionID
			case "resume":
				if state.held == "" || r.EndInclusive != state.end {
					return nil, ErrAdmissionConflict
				}
				state.held = ""
			default:
				return nil, ErrAdmissionConflict
			}
			state.tip = r
			state.chain = append(state.chain, id)
		}
		states = append(states, state)
	}
	if len(visited) != len(rows) {
		return nil, ErrAdmissionConflict
	}
	for i, x := range states {
		for _, y := range states[i+1:] {
			if x.root.Subject == y.root.Subject && rangesOverlap(x.root.StartAfter, x.end, y.root.StartAfter, y.end) {
				return nil, ErrAdmissionConflict
			}
		}
	}
	return states, nil
}

func sameAdmissionMetadata(a, b sourceAdmissionRow) bool {
	var x, y []string
	if json.Unmarshal(a.SchemaAllowlist, &x) != nil || json.Unmarshal(b.SchemaAllowlist, &y) != nil {
		return false
	}
	slices.Sort(x)
	slices.Sort(y)
	return a.ContractID == b.ContractID && a.ContractVersion == b.ContractVersion && bytes.Equal(a.ContractDigest, b.ContractDigest) &&
		a.ScopeRef == b.ScopeRef && a.Purpose == b.Purpose && a.StartAfter == b.StartAfter && slices.Equal(x, y)
}
func rangesOverlap(a int64, ae sql.NullInt64, b int64, be sql.NullInt64) bool {
	if (ae.Valid && ae.Int64 == a) || (be.Valid && be.Int64 == b) {
		return false
	}
	return (!ae.Valid || b < ae.Int64) && (!be.Valid || a < be.Int64)
}
func rowContract(r sourceAdmissionRow) AdmissionContract {
	var d [sha256.Size]byte
	copy(d[:], r.ContractDigest)
	return AdmissionContract{r.ContractID, r.ContractVersion, d}
}

func (a *SourceAdmissions) highWater(ctx context.Context, s SourceStream) (int64, int64, error) {
	var h struct{ Count, Revision, Sequence, IntegrationCount int64 }
	err := a.tx.WithContext(ctx).Raw(`SELECT count(*) AS count,coalesce(max(aggregate_version),0) AS revision,
 coalesce(max(integration_sequence),0) AS sequence,count(integration_sequence) AS integration_count
 FROM eventstore.events WHERE owner_service=? AND aggregate_type=? AND aggregate_id=?::uuid`, string(s.owner), s.aggregateType, s.aggregateID).Scan(&h).Error
	if err != nil {
		return 0, 0, err
	}
	if h.Count != h.Revision || h.Sequence != h.IntegrationCount {
		return 0, 0, eventstore.ErrCorruptHistory
	}
	return h.Sequence, h.Count, nil
}

func (a *SourceAdmissions) Admit(ctx context.Context, admission SourceAdmission) error {
	in := admission.input
	if err := a.check(in.Stream); err != nil {
		return err
	}
	if _, err := NewSourceAdmission(in); err != nil {
		return err
	}
	states, _, err := a.load(ctx, in.Stream)
	if err != nil {
		return err
	}
	h, count, err := a.highWater(ctx, in.Stream)
	if err != nil {
		return err
	}
	if in.Bootstrap.Checkpoint.Int64() != h || (h == 0 && (count != 0 || !evidence(in.Bootstrap.EmptyStreamRef))) {
		return ErrAdmissionCheckpoint
	}
	for _, state := range states {
		if state.root.Subject == string(in.Destination) && rangesOverlap(h, sql.NullInt64{}, state.root.StartAfter, state.end) {
			return ErrAdmissionConflict
		}
	}
	schemas, _ := json.Marshal(in.Schemas)
	// Preserve zero-stream proof alongside the bootstrap reference in immutable
	// JSON evidence. The schema treats this column as an opaque reference.
	bootstrap, _ := json.Marshal(in.Bootstrap)
	return a.tx.WithContext(ctx).Exec(`INSERT INTO eventstore.messaging_admissions
 (admission_id,namespace,source_owner,aggregate_type,aggregate_id,subject,action,contract_id,contract_version,contract_digest,
 schema_allowlist,authority_ref,scope_ref,purpose,bootstrap_ref,start_after)
 VALUES (?::uuid,'source-recipient',?,?,?::uuid,?,'admit',?,?,?,?::jsonb,?,?,?,?,?)`, in.ID, string(in.Stream.owner), in.Stream.aggregateType, in.Stream.aggregateID, string(in.Destination),
		in.Contract.ID, in.Contract.Version, in.Contract.Digest[:], string(schemas), in.AuthorityRef, in.ScopeRef, in.Purpose, string(bootstrap), h).Error
}

func (a *SourceAdmissions) Change(ctx context.Context, s SourceStream, c SourceAdmissionChange) error {
	if err := a.check(s); err != nil {
		return err
	}
	if !admissionUUID(c.ID) || !admissionUUID(c.PriorID) || !evidence(c.AuthorityRef) || !evidence(c.EvidenceRef) ||
		(c.Action != AdmissionClose && c.Action != AdmissionHold && c.Action != AdmissionResume) ||
		(c.Action == AdmissionResume && !evidence(c.RetainedWorkAuthorityRef)) {
		return ErrInvalidAdmission
	}
	states, _, err := a.load(ctx, s)
	if err != nil {
		return err
	}
	h, _, err := a.highWater(ctx, s)
	if err != nil {
		return err
	}
	if c.Checkpoint.Int64() != h {
		return ErrAdmissionCheckpoint
	}
	var selected *effectiveAdmission
	for i := range states {
		if states[i].tip.AdmissionID == c.PriorID {
			selected = &states[i]
		}
	}
	if selected == nil {
		return ErrAdmissionConflict
	}
	state := *selected
	end := state.end
	switch c.Action {
	case AdmissionClose:
		if end.Valid || h < state.root.StartAfter {
			return ErrAdmissionConflict
		}
		end = sql.NullInt64{Int64: h, Valid: true}
	case AdmissionHold:
		if state.held != "" {
			return ErrAdmissionConflict
		}
	case AdmissionResume:
		if state.held == "" {
			return ErrAdmissionConflict
		}
	}
	ref, _ := json.Marshal(struct {
		EvidenceRef, RetainedWorkAuthorityRef string
		Checkpoint                            events.Revision
	}{c.EvidenceRef, c.RetainedWorkAuthorityRef, c.Checkpoint})
	r := state.root
	err = a.tx.WithContext(ctx).Exec(`INSERT INTO eventstore.messaging_admissions
 (admission_id,namespace,source_owner,aggregate_type,aggregate_id,subject,action,prior_admission_id,contract_id,contract_version,contract_digest,
 schema_allowlist,authority_ref,scope_ref,purpose,bootstrap_ref,start_after,end_inclusive)
 VALUES (?::uuid,'source-recipient',?,?,?::uuid,?,?,?::uuid,?,?,?,?::jsonb,?,?,?,?,?,?)`, c.ID, string(s.owner), s.aggregateType, s.aggregateID, r.Subject, string(c.Action), c.PriorID,
		r.ContractID, r.ContractVersion, r.ContractDigest, string(r.SchemaAllowlist), c.AuthorityRef, r.ScopeRef, r.Purpose, string(ref), r.StartAfter, end).Error
	if err != nil {
		return err
	}
	// Different holds may belong to migration/security processes. Never overwrite
	// or release those; the relay must enforce both this chain and row holds.
	if c.Action == AdmissionHold {
		return a.tx.WithContext(ctx).Exec(`UPDATE eventstore.outbox_deliveries SET hold_ref=?
 WHERE admission_id=?::uuid AND sent_at IS NULL AND hold_ref IS NULL`, "source-admission:"+c.ID, r.AdmissionID).Error
	}
	if c.Action == AdmissionResume {
		return a.tx.WithContext(ctx).Exec(`UPDATE eventstore.outbox_deliveries SET hold_ref=NULL
 WHERE admission_id=?::uuid AND sent_at IS NULL AND hold_ref=?`, r.AdmissionID, "source-admission:"+state.held).Error
	}
	return nil
}

// Resolve reads every effective admission under the already-held source fence.
// It requires an envelope just appended in this transaction, including earlier
// positions in a multi-event batch. Committed history cannot be re-planned.
// Payload schema validation/serialization and parent+children insertion are the
// T-919 writer's responsibility; no broker action belongs in this transaction.
func (a *SourceAdmissions) Resolve(ctx context.Context, rule SourcePlanRule, e events.Envelope) (DeliveryPlan, error) {
	if err := a.check(rule.stream); err != nil {
		return DeliveryPlan{}, err
	}
	if !evidence(rule.authority) || len(rule.schemas) == 0 || e.Validate() != nil || e.Owner() != rule.stream.owner ||
		e.AggregateType() != rule.stream.aggregateType || e.AggregateID() != rule.stream.aggregateID {
		return DeliveryPlan{}, ErrInvalidAdmission
	}
	if !slices.Contains(rule.schemas, e.EventType()) {
		return DeliveryPlan{}, ErrAdmissionSchema
	}
	states, hash, err := a.load(ctx, rule.stream)
	if err != nil {
		return DeliveryPlan{}, err
	}
	_, _, err = a.highWater(ctx, rule.stream)
	if err != nil {
		return DeliveryPlan{}, err
	}
	seq := e.IntegrationSequence().Int64()
	var count int64
	if err := a.tx.WithContext(ctx).Raw(`SELECT count(*) FROM eventstore.events WHERE event_id=?::uuid AND owner_service=?
 AND aggregate_type=? AND aggregate_id=?::uuid AND integration_sequence=? AND event_type=? AND schema_version=?
 AND aggregate_version=? AND xmin=pg_current_xact_id()::xid`, e.EventID(), string(a.owner), rule.stream.aggregateType, rule.stream.aggregateID, seq, e.EventType(), e.SchemaVersion(), e.AggregateVersion().Int64()).Scan(&count).Error; err != nil {
		return DeliveryPlan{}, err
	}
	if count != 1 {
		return DeliveryPlan{}, ErrAdmissionCheckpoint
	}
	plan := DeliveryPlan{stream: rule.stream, eventID: e.EventID(), eventType: e.EventType(), sequence: e.IntegrationSequence(), authority: rule.authority, issuer: a, evidenceHash: hash, recipients: make([]PlanRecipient, 0)}
	for _, state := range states {
		if seq <= state.root.StartAfter || (state.end.Valid && seq > state.end.Int64) {
			continue
		}
		if state.held != "" {
			return DeliveryPlan{}, ErrAdmissionHeld
		}
		var schemas []string
		_ = json.Unmarshal(state.root.SchemaAllowlist, &schemas)
		if !slices.Contains(schemas, e.EventType()) {
			return DeliveryPlan{}, ErrAdmissionSchema
		}
		destination := events.Owner(state.root.Subject)
		if len(route(e, destination)) > 255 {
			return DeliveryPlan{}, ErrAdmissionSchema
		}
		plan.recipients = append(plan.recipients, PlanRecipient{destination, state.root.AdmissionID, rowContract(state.root)})
	}
	if len(plan.recipients) == 0 {
		if !evidence(rule.emptyAuthority) {
			return DeliveryPlan{}, ErrMissingRecipientPlan
		}
		plan.authority = rule.emptyAuthority
	}
	slices.SortFunc(plan.recipients, func(x, y PlanRecipient) int { return strings.Compare(string(x.Destination), string(y.Destination)) })
	return plan, nil
}

// ValidatePlan is the T-919 same-transaction handoff guard. Call immediately
// before persistence if resolution and INSERT are separate adapter steps.
// Changing any admission evidence in between invalidates the candidate.
func (a *SourceAdmissions) ValidatePlan(ctx context.Context, p DeliveryPlan) error {
	if err := a.check(p.stream); err != nil {
		return err
	}
	if p.issuer != a || p.eventID == "" || p.sequence.IsZero() || !evidence(p.authority) {
		return ErrInvalidAdmission
	}
	_, hash, err := a.load(ctx, p.stream)
	if err != nil {
		return err
	}
	if hash != p.evidenceHash {
		return ErrAdmissionConflict
	}
	var count int64
	if err := a.tx.WithContext(ctx).Raw(`SELECT count(*) FROM eventstore.events WHERE event_id=?::uuid AND owner_service=?
 AND aggregate_type=? AND aggregate_id=?::uuid AND integration_sequence=? AND event_type=? AND xmin=pg_current_xact_id()::xid`, p.eventID, string(p.stream.owner), p.stream.aggregateType, p.stream.aggregateID, p.sequence.Int64(), p.eventType).Scan(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrAdmissionCheckpoint
	}
	return nil
}

func admissionUUID(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u != uuid.Nil && u.String() == id
}

func evidence(s string) bool { return strings.TrimSpace(s) != "" && !strings.ContainsRune(s, 0) }
func validContract(c AdmissionContract) bool {
	return evidence(c.ID) && c.Version > 0 && c.Digest != [sha256.Size]byte{}
}
func checkedSchemas(owner events.Owner, schemas []string) ([]string, error) {
	if len(schemas) == 0 {
		return nil, ErrInvalidAdmission
	}
	out := slices.Clone(schemas)
	slices.Sort(out)
	for i, s := range out {
		if !admissionSchema.MatchString(s) || !strings.HasPrefix(s, string(owner)+".") || (i > 0 && s == out[i-1]) {
			return nil, ErrInvalidAdmission
		}
		if _, err := strconv.ParseUint(s[strings.LastIndex(s, ".v")+2:], 10, 32); err != nil {
			return nil, ErrInvalidAdmission
		}
	}
	return out, nil
}
