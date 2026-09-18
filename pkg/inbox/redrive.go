package inbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
)

var (
	ErrInvalidRedrive  = errors.New("inbox: invalid redrive request")
	ErrRedriveConflict = errors.New("inbox: redrive request conflicts with retained evidence")
	ErrRedriveFailed   = errors.New("inbox: ordinary redrive path failed")
)

type RedriveRequestInput struct {
	EvidenceID, ActionID, RequestID, PriorActionID string
	ActorRef, AuthorityRef, ScopeRef, PurposeRef   string
	RepairRef, ManifestRef                         string
	ManifestHash                                   [32]byte
	IntendedPath                                   QuarantineStage
	IntendedConsumer                               string
	RequiredFences                                 []eventstore.FenceRequest
}

type RedriveResultInput struct {
	ActionID, RequestID string
}

// RedriveAuthorization is presented to the injected current-authority port
// before a request is appended and again before protected bytes are restored.
// References are evidence inputs, never permissions by their mere presence.
type RedriveAuthorization struct {
	EvidenceID, Stage, OwnerService string
	ConsumerName, JobEventID        string
	ActorRef, AuthorityRef          string
	ScopeRef, PurposeRef, RepairRef string
}

type RedriveAuthorizer interface {
	Authorize(context.Context, RedriveAuthorization) error
}

type ReceiptKind string

const (
	ReceiptInbox   ReceiptKind = "inbox"
	ReceiptCustody ReceiptKind = "custody"
	ReceiptJob     ReceiptKind = "job"
)

// OrdinaryReceipt must come from the ordinary intake/inbox/job path. The SQL
// evidence guard independently checks the cited durable row before accepting a
// result action. A returned struct alone can never manufacture progress.
type OrdinaryReceipt struct {
	Kind         ReceiptKind
	ConsumerName string
	EventID      string
	EnvelopeHash [32]byte
}

type RedrivePayload struct {
	OriginalBytes []byte
	RepairRef     string
	ManifestRef   string
	ManifestHash  [32]byte
}

type OrdinaryRedrive interface {
	Apply(context.Context, RedrivePayload) (OrdinaryReceipt, error)
}

type OrdinaryRedriveFunc func(context.Context, RedrivePayload) (OrdinaryReceipt, error)

func (f OrdinaryRedriveFunc) Apply(ctx context.Context, p RedrivePayload) (OrdinaryReceipt, error) {
	return f(ctx, p)
}

type RedriveOutcome struct {
	Code      string
	Receipt   OrdinaryReceipt
	Duplicate bool
}

type RedriveTicket struct{ value *redriveTicket }
type redriveTicket struct {
	request  RedriveRequestInput
	evidence redriveEvidence
}

// Redriver journals authorization before touching protected content. It has no
// broker channel and cannot bypass the injected ordinary delivery path.
type Redriver struct {
	db         *gorm.DB
	owner      events.Owner
	security   EvidenceSecurity
	authorizer RedriveAuthorizer
	runner     *eventstore.Transactions[*gorm.DB]
}

func NewRedriver(db *gorm.DB, owner events.Owner, security EvidenceSecurity, authorizer RedriveAuthorizer) (*Redriver, error) {
	if db == nil || db.Error != nil || !owner.Valid() || security == nil || authorizer == nil {
		return nil, ErrInvalidRedrive
	}
	runner, err := eventstore.NewTransactions(db, func(tx *gorm.DB) (*gorm.DB, error) { return tx, nil })
	if err != nil {
		return nil, err
	}
	return &Redriver{db: db, owner: owner, security: security, authorizer: authorizer, runner: runner}, nil
}

// Request appends one authorized redrive-request to the current evidence tip.
// Exact replay returns the same ticket; a branch or changed request conflicts.
func (r *Redriver) Request(ctx context.Context, in RedriveRequestInput) (RedriveTicket, error) {
	if r == nil || r.runner == nil || !validRedriveRequest(in) {
		return RedriveTicket{}, ErrInvalidRedrive
	}
	e, err := r.readEvidence(ctx, r.db, in.EvidenceID)
	if err != nil {
		return RedriveTicket{}, err
	}
	if !r.compatible(e, in) {
		return RedriveTicket{}, ErrRedriveConflict
	}
	if err := r.authorizer.Authorize(ctx, authorization(e, in)); err != nil {
		return RedriveTicket{}, err
	}
	var retained redriveEvidence
	err = r.runner.Run(ctx, func(tx *gorm.DB) error {
		if err := requireQuarantineReady(ctx, tx, r.owner); err != nil {
			return err
		}
		var err error
		retained, err = r.readEvidence(ctx, tx, in.EvidenceID)
		if err != nil {
			return err
		}
		if !r.compatible(retained, in) {
			return ErrRedriveConflict
		}
		return appendRedriveRequest(ctx, tx, in)
	})
	if err != nil {
		return RedriveTicket{}, err
	}
	in.RequiredFences = append([]eventstore.FenceRequest(nil), in.RequiredFences...)
	return RedriveTicket{&redriveTicket{request: in, evidence: retained}}, nil
}

// Execute restores exact original bytes outside SQL and submits them to an
// injected ordinary path. Crash after Apply is safe: a retry reaches ordinary
// dedup, while an already committed result action is returned without Apply.
func (r *Redriver) Execute(ctx context.Context, ticket RedriveTicket, result RedriveResultInput, path OrdinaryRedrive) (RedriveOutcome, error) {
	if r == nil || ticket.value == nil || path == nil || !validResult(result) {
		return RedriveOutcome{}, ErrInvalidRedrive
	}
	if out, found, err := r.existingResult(ctx, ticket, result); err != nil || found {
		return out, err
	}
	e, err := r.readEvidence(ctx, r.db, ticket.value.request.EvidenceID)
	if err != nil {
		return RedriveOutcome{}, err
	}
	if err := r.authorizer.Authorize(ctx, authorization(e, ticket.value.request)); err != nil {
		journalErr := r.appendResult(ctx, ticket, result, "denied", OrdinaryReceipt{})
		return RedriveOutcome{Code: "denied"}, errors.Join(err, journalErr)
	}
	raw, err := r.restore(ctx, e)
	if err != nil {
		journalErr := r.appendResult(ctx, ticket, result, "failed", OrdinaryReceipt{})
		return RedriveOutcome{Code: "failed"}, errors.Join(err, journalErr)
	}
	defer clear(raw)
	receipt, applyErr := path.Apply(ctx, RedrivePayload{OriginalBytes: bytes.Clone(raw), RepairRef: ticket.value.request.RepairRef, ManifestRef: ticket.value.request.ManifestRef, ManifestHash: ticket.value.request.ManifestHash})
	if applyErr != nil {
		journalErr := r.appendResult(ctx, ticket, result, "failed", OrdinaryReceipt{})
		return RedriveOutcome{Code: "failed"}, errors.Join(ErrRedriveFailed, applyErr, journalErr)
	}
	if !validReceipt(ticket.value.request, receipt) {
		return RedriveOutcome{}, ErrInvalidRedrive
	}
	if err := r.appendResult(ctx, ticket, result, "accepted", receipt); err != nil {
		return RedriveOutcome{}, err
	}
	return RedriveOutcome{Code: "accepted", Receipt: receipt}, nil
}

// ReleaseCustody restores and verifies the original protected bytes, then under
// the receiver fence atomically appends a released result and clears only this
// evidence's quarantine reference. Other holds remain. It does not execute or
// complete the job; the ordinary T-923 path does that later.
func (r *Redriver) ReleaseCustody(ctx context.Context, ticket RedriveTicket, result RedriveResultInput) (RedriveOutcome, error) {
	if r == nil || ticket.value == nil || !validResult(result) || ticket.value.request.IntendedPath != QuarantineCustodyHandler {
		return RedriveOutcome{}, ErrInvalidRedrive
	}
	if out, found, err := r.existingResult(ctx, ticket, result); err != nil || found {
		return out, err
	}
	e, err := r.readEvidence(ctx, r.db, ticket.value.request.EvidenceID)
	if err != nil {
		return RedriveOutcome{}, err
	}
	if err := r.authorizer.Authorize(ctx, authorization(e, ticket.value.request)); err != nil {
		journalErr := r.appendResult(ctx, ticket, result, "denied", OrdinaryReceipt{})
		return RedriveOutcome{Code: "denied"}, errors.Join(err, journalErr)
	}
	raw, err := r.restore(ctx, e)
	if err != nil {
		journalErr := r.appendResult(ctx, ticket, result, "failed", OrdinaryReceipt{})
		return RedriveOutcome{Code: "failed"}, errors.Join(err, journalErr)
	}
	clear(raw)
	err = r.runner.Run(ctx, func(tx *gorm.DB) error {
		if err := requireQuarantineReady(ctx, tx, r.owner); err != nil {
			return err
		}
		current, err := r.readEvidence(ctx, tx, e.EvidenceID)
		if err != nil || !sameRedriveEvidence(e, current) {
			if err != nil {
				return err
			}
			return ErrRedriveConflict
		}
		stream := current.stream()
		if stream == nil {
			return ErrRedriveConflict
		}
		receiver, err := eventstore.ReceiverFence(r.owner, stream.SourceOwner, stream.AggregateType, stream.AggregateID)
		if err != nil {
			return err
		}
		requests := append([]eventstore.FenceRequest{receiver}, ticket.value.request.RequiredFences...)
		if _, err := eventstore.PrepareFences(ctx, tx, r.owner, eventstore.SharedFence, requests...); err != nil {
			return err
		}
		var job struct {
			CompletedAt   *time.Time
			QuarantineRef *string
		}
		read := tx.WithContext(ctx).Raw(`SELECT completed_at,quarantine_ref FROM eventstore.dispatch_jobs
			WHERE consumer_name=? AND event_id=?::uuid FOR UPDATE`, current.ConsumerName, current.JobEventID).Scan(&job)
		if read.Error != nil {
			return read.Error
		}
		want := "quarantine:" + current.EvidenceID
		if read.RowsAffected != 1 || job.CompletedAt != nil || job.QuarantineRef == nil || *job.QuarantineRef != want {
			return ErrRedriveConflict
		}
		if err := appendResultTx(ctx, tx, ticket.value.request, result, "released", OrdinaryReceipt{}); err != nil {
			return err
		}
		update := tx.WithContext(ctx).Exec(`UPDATE eventstore.dispatch_jobs SET quarantine_ref=NULL
			WHERE consumer_name=? AND event_id=?::uuid AND completed_at IS NULL AND quarantine_ref=?`, current.ConsumerName, current.JobEventID, want)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrRedriveConflict
		}
		return nil
	})
	if err != nil {
		return RedriveOutcome{}, err
	}
	return RedriveOutcome{Code: "released"}, nil
}

type redriveEvidence struct {
	EvidenceID, CaptureRequestID, Stage, OwnerService string
	QueueRef, ConsumerName, SourceOwner               *string
	AggregateType, AggregateID, JobEventID            *string
	RawSha256, SealedCiphertext, CiphertextSha256     []byte
	RawByteLength                                     int64
	SealFormatRef, SealKeyRef, CapturePolicyRef       string
}

func (e redriveEvidence) stream() *QuarantineStream {
	if e.SourceOwner == nil || e.AggregateType == nil || e.AggregateID == nil {
		return nil
	}
	return &QuarantineStream{SourceOwner: events.Owner(*e.SourceOwner), AggregateType: *e.AggregateType, AggregateID: *e.AggregateID}
}

func (r *Redriver) readEvidence(ctx context.Context, db *gorm.DB, id string) (redriveEvidence, error) {
	var e redriveEvidence
	read := db.WithContext(ctx).Table("eventstore.quarantine_evidence").Where("evidence_id=?::uuid", id).Take(&e)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return e, ErrInvalidRedrive
	}
	return e, read.Error
}

func (r *Redriver) compatible(e redriveEvidence, in RedriveRequestInput) bool {
	if e.OwnerService != string(r.owner) || e.EvidenceID != in.EvidenceID || e.Stage != string(in.IntendedPath) {
		return false
	}
	consumer := ""
	if e.ConsumerName != nil {
		consumer = *e.ConsumerName
	}
	return consumer == in.IntendedConsumer
}

func authorization(e redriveEvidence, in RedriveRequestInput) RedriveAuthorization {
	consumer, job := "", ""
	if e.ConsumerName != nil {
		consumer = *e.ConsumerName
	}
	if e.JobEventID != nil {
		job = *e.JobEventID
	}
	return RedriveAuthorization{EvidenceID: e.EvidenceID, Stage: e.Stage, OwnerService: e.OwnerService, ConsumerName: consumer, JobEventID: job,
		ActorRef: in.ActorRef, AuthorityRef: in.AuthorityRef, ScopeRef: in.ScopeRef, PurposeRef: in.PurposeRef, RepairRef: in.RepairRef}
}

func validRedriveRequest(in RedriveRequestInput) bool {
	for _, id := range []string{in.EvidenceID, in.ActionID, in.RequestID, in.PriorActionID} {
		if !canonicalQuarantineID(id) {
			return false
		}
	}
	for _, ref := range []string{in.ActorRef, in.AuthorityRef, in.ScopeRef, in.PurposeRef, in.RepairRef, in.ManifestRef} {
		if !safeRef(ref) {
			return false
		}
	}
	if in.ManifestHash == ([32]byte{}) {
		return false
	}
	if in.IntendedPath != QuarantineIntake && !safeRef(in.IntendedConsumer) {
		return false
	}
	return in.IntendedPath == QuarantineIntake || in.IntendedPath == QuarantineDirectHandler || in.IntendedPath == QuarantineCustodyHandler
}

func validResult(in RedriveResultInput) bool {
	return canonicalQuarantineID(in.ActionID) && canonicalQuarantineID(in.RequestID)
}

func appendRedriveRequest(ctx context.Context, tx *gorm.DB, in RedriveRequestInput) error {
	if err := tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "justixauto:quarantine-chain:"+in.EvidenceID).Error; err != nil {
		return err
	}
	want := redriveActionRow{ActionID: in.ActionID, EvidenceID: in.EvidenceID, RequestID: in.RequestID, PriorActionID: &in.PriorActionID,
		Action: "redrive-request", EvidenceFormatVersion: 1, ActorRef: in.ActorRef, AuthorityRef: in.AuthorityRef, ScopeRef: in.ScopeRef,
		PurposeRef: in.PurposeRef, RepairRef: in.RepairRef, IntendedPath: string(in.IntendedPath), IntendedConsumer: nullableQuarantine(in.IntendedConsumer),
		ManifestRef: in.ManifestRef, ManifestSha256: bytes.Clone(in.ManifestHash[:]), OutcomeCode: "requested"}
	var existing redriveActionRow
	find := tx.WithContext(ctx).Table("eventstore.quarantine_actions").Where("action_id=?::uuid OR request_id=?::uuid", in.ActionID, in.RequestID).Take(&existing)
	if find.Error == nil {
		if !sameRedriveAction(existing, want) {
			return ErrRedriveConflict
		}
		return nil
	}
	if !errors.Is(find.Error, gorm.ErrRecordNotFound) {
		return find.Error
	}
	var tip string
	read := tx.WithContext(ctx).Raw(`SELECT a.action_id FROM eventstore.quarantine_actions a
		WHERE a.evidence_id=?::uuid AND NOT EXISTS (SELECT FROM eventstore.quarantine_actions n WHERE n.prior_action_id=a.action_id)`, in.EvidenceID).Scan(&tip)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 || tip != in.PriorActionID {
		return ErrRedriveConflict
	}
	return tx.WithContext(ctx).Table("eventstore.quarantine_actions").Create(&want).Error
}

type redriveActionRow struct {
	ActionID, EvidenceID, RequestID                  string
	PriorActionID                                    *string
	Action                                           string
	EvidenceFormatVersion                            int
	ActorRef, AuthorityRef                           string
	ScopeRef, PurposeRef                             string
	RepairRef, IntendedPath                          string
	IntendedConsumer                                 *string
	ManifestRef                                      string
	ManifestSha256                                   []byte
	AcceptedEventID                                  *string
	AcceptedEventHash                                []byte
	ReceiptKind, ReceiptConsumerName, ReceiptEventID *string
	OutcomeCode                                      string
}

func resultRow(req RedriveRequestInput, result RedriveResultInput, code string, receipt OrdinaryReceipt) redriveActionRow {
	r := redriveActionRow{ActionID: result.ActionID, EvidenceID: req.EvidenceID, RequestID: result.RequestID, PriorActionID: &req.ActionID,
		Action: "redrive-result", EvidenceFormatVersion: 1, ActorRef: req.ActorRef, AuthorityRef: req.AuthorityRef, ScopeRef: req.ScopeRef,
		PurposeRef: req.PurposeRef, RepairRef: req.RepairRef, IntendedPath: string(req.IntendedPath), IntendedConsumer: nullableQuarantine(req.IntendedConsumer),
		ManifestRef: req.ManifestRef, ManifestSha256: bytes.Clone(req.ManifestHash[:]), OutcomeCode: code}
	if code == "accepted" {
		kind, eventID := string(receipt.Kind), receipt.EventID
		r.AcceptedEventID, r.AcceptedEventHash = &eventID, bytes.Clone(receipt.EnvelopeHash[:])
		r.ReceiptKind, r.ReceiptEventID = &kind, &eventID
		if receipt.ConsumerName != "" {
			r.ReceiptConsumerName = nullableQuarantine(receipt.ConsumerName)
		}
	}
	return r
}

func (r *Redriver) appendResult(ctx context.Context, ticket RedriveTicket, result RedriveResultInput, code string, receipt OrdinaryReceipt) error {
	return r.runner.Run(ctx, func(tx *gorm.DB) error {
		if err := requireQuarantineReady(ctx, tx, r.owner); err != nil {
			return err
		}
		return appendResultTx(ctx, tx, ticket.value.request, result, code, receipt)
	})
}

func appendResultTx(ctx context.Context, tx *gorm.DB, req RedriveRequestInput, result RedriveResultInput, code string, receipt OrdinaryReceipt) error {
	if err := tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "justixauto:quarantine-chain:"+req.EvidenceID).Error; err != nil {
		return err
	}
	want := resultRow(req, result, code, receipt)
	var existing redriveActionRow
	find := tx.WithContext(ctx).Table("eventstore.quarantine_actions").Where("action_id=?::uuid OR request_id=?::uuid", result.ActionID, result.RequestID).Take(&existing)
	if find.Error == nil {
		if !sameRedriveAction(existing, want) {
			return ErrRedriveConflict
		}
		return nil
	}
	if !errors.Is(find.Error, gorm.ErrRecordNotFound) {
		return find.Error
	}
	var tip string
	read := tx.WithContext(ctx).Raw(`SELECT a.action_id FROM eventstore.quarantine_actions a
		WHERE a.evidence_id=?::uuid AND NOT EXISTS (SELECT FROM eventstore.quarantine_actions n WHERE n.prior_action_id=a.action_id)`, req.EvidenceID).Scan(&tip)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 || tip != req.ActionID {
		return ErrRedriveConflict
	}
	return tx.WithContext(ctx).Table("eventstore.quarantine_actions").Create(&want).Error
}

func (r *Redriver) existingResult(ctx context.Context, ticket RedriveTicket, result RedriveResultInput) (RedriveOutcome, bool, error) {
	var row redriveActionRow
	read := r.db.WithContext(ctx).Table("eventstore.quarantine_actions").Where("action_id=?::uuid OR request_id=?::uuid", result.ActionID, result.RequestID).Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return RedriveOutcome{}, false, nil
	}
	if read.Error != nil {
		return RedriveOutcome{}, false, read.Error
	}
	wantBase := resultRow(ticket.value.request, result, row.OutcomeCode, receiptFromRow(row))
	if !sameRedriveAction(row, wantBase) {
		return RedriveOutcome{}, true, ErrRedriveConflict
	}
	return RedriveOutcome{Code: row.OutcomeCode, Receipt: receiptFromRow(row), Duplicate: true}, true, nil
}

func receiptFromRow(row redriveActionRow) OrdinaryReceipt {
	var r OrdinaryReceipt
	if row.ReceiptKind != nil {
		r.Kind = ReceiptKind(*row.ReceiptKind)
	}
	if row.ReceiptConsumerName != nil {
		r.ConsumerName = *row.ReceiptConsumerName
	}
	if row.ReceiptEventID != nil {
		r.EventID = *row.ReceiptEventID
	}
	copy(r.EnvelopeHash[:], row.AcceptedEventHash)
	return r
}

func validReceipt(req RedriveRequestInput, receipt OrdinaryReceipt) bool {
	if !canonicalQuarantineID(receipt.EventID) || receipt.EnvelopeHash == ([32]byte{}) {
		return false
	}
	switch req.IntendedPath {
	case QuarantineIntake:
		return receipt.Kind == ReceiptCustody && receipt.ConsumerName == ""
	case QuarantineDirectHandler:
		return receipt.Kind == ReceiptInbox && receipt.ConsumerName == req.IntendedConsumer
	case QuarantineCustodyHandler:
		return receipt.Kind == ReceiptJob && receipt.ConsumerName == req.IntendedConsumer
	default:
		return false
	}
}

func sameRedriveAction(a, b redriveActionRow) bool {
	return a.ActionID == b.ActionID && a.EvidenceID == b.EvidenceID && a.RequestID == b.RequestID && equalStringPtr(a.PriorActionID, b.PriorActionID) &&
		a.Action == b.Action && a.EvidenceFormatVersion == b.EvidenceFormatVersion && a.ActorRef == b.ActorRef && a.AuthorityRef == b.AuthorityRef &&
		a.ScopeRef == b.ScopeRef && a.PurposeRef == b.PurposeRef && a.RepairRef == b.RepairRef && a.IntendedPath == b.IntendedPath &&
		equalStringPtr(a.IntendedConsumer, b.IntendedConsumer) && a.ManifestRef == b.ManifestRef && bytes.Equal(a.ManifestSha256, b.ManifestSha256) &&
		equalStringPtr(a.AcceptedEventID, b.AcceptedEventID) && bytes.Equal(a.AcceptedEventHash, b.AcceptedEventHash) &&
		equalStringPtr(a.ReceiptKind, b.ReceiptKind) && equalStringPtr(a.ReceiptConsumerName, b.ReceiptConsumerName) && equalStringPtr(a.ReceiptEventID, b.ReceiptEventID) &&
		a.OutcomeCode == b.OutcomeCode
}

func (r *Redriver) restore(ctx context.Context, e redriveEvidence) ([]byte, error) {
	cipherHash := sha256.Sum256(e.SealedCiphertext)
	if len(e.RawSha256) != sha256.Size || len(e.CiphertextSha256) != sha256.Size || !bytes.Equal(e.CiphertextSha256, cipherHash[:]) {
		return nil, ErrRedriveConflict
	}
	raw, err := r.security.Restore(ctx, SealedEvidence{Ciphertext: bytes.Clone(e.SealedCiphertext), FormatRef: e.SealFormatRef, KeyRef: e.SealKeyRef, CapturePolicyRef: e.CapturePolicyRef})
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) != e.RawByteLength {
		clear(raw)
		return nil, ErrRedriveConflict
	}
	h := sha256.Sum256(raw)
	if !bytes.Equal(h[:], e.RawSha256) {
		clear(raw)
		return nil, ErrRedriveConflict
	}
	return raw, nil
}

func sameRedriveEvidence(a, b redriveEvidence) bool {
	return a.EvidenceID == b.EvidenceID && a.CaptureRequestID == b.CaptureRequestID && a.Stage == b.Stage && a.OwnerService == b.OwnerService &&
		equalStringPtr(a.QueueRef, b.QueueRef) && equalStringPtr(a.ConsumerName, b.ConsumerName) && equalStringPtr(a.SourceOwner, b.SourceOwner) &&
		equalStringPtr(a.AggregateType, b.AggregateType) && equalStringPtr(a.AggregateID, b.AggregateID) && equalStringPtr(a.JobEventID, b.JobEventID) &&
		bytes.Equal(a.RawSha256, b.RawSha256) && a.RawByteLength == b.RawByteLength && bytes.Equal(a.SealedCiphertext, b.SealedCiphertext) &&
		bytes.Equal(a.CiphertextSha256, b.CiphertextSha256) && a.SealFormatRef == b.SealFormatRef && a.SealKeyRef == b.SealKeyRef && a.CapturePolicyRef == b.CapturePolicyRef
}

func (t RedriveTicket) String() string {
	if t.value == nil {
		return "redrive ticket <invalid>"
	}
	return fmt.Sprintf("redrive ticket %s", t.value.request.ActionID)
}
