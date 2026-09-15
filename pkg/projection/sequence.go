// Package projection implements owner-local sequence mechanics. Domain and app
// packages consume their own typed ports, not these SQL adapters.
package projection

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
)

var (
	ErrInvalid             = errors.New("projection: invalid contract or evidence")
	ErrConflict            = errors.New("projection: retained evidence conflict")
	ErrReconciliationHold  = errors.New("projection: checkpoint reconciliation required")
	ErrGap                 = errors.New("projection: missing integration positions")
	ErrSeparateTransaction = errors.New("projection: gap evidence requires a separate transaction")
)

// ContractInput is deployment wiring, not a user request. Generation is the
// exact retained admission label for both projection and process consumers.
// Generation/guard/source prerequisites must be declared by the owner here and
// included in PrepareFences before any head/job/inbox/effect row is acquired.
type ContractInput struct {
	LocalOwner, SourceOwner                                events.Owner
	Consumer, AggregateType, AggregateID, Kind, Generation string
	ContractID                                             string
	ContractVersion                                        uint32
	ContractDigest                                         [32]byte
	RequiredFences                                         []eventstore.FenceRequest
}

type Contract struct{ value *ContractInput }

func nonempty(s string) bool {
	return strings.TrimSpace(s) != "" && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
func generationLabel(s string) bool {
	return s != "" && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
func canonicalID(s string) bool {
	u, e := uuid.Parse(s)
	return e == nil && u != uuid.Nil && u.String() == s
}

func NewContract(in ContractInput) (Contract, error) {
	if !in.LocalOwner.Valid() || !in.SourceOwner.Valid() || !nonempty(in.Consumer) || !generationLabel(in.Generation) || !nonempty(in.ContractID) || in.ContractVersion == 0 || in.ContractDigest == ([32]byte{}) || (in.Kind != "projection" && in.Kind != "process") {
		return Contract{}, ErrInvalid
	}
	if _, err := eventstore.ReceiverFence(in.LocalOwner, in.SourceOwner, in.AggregateType, in.AggregateID); err != nil {
		return Contract{}, err
	}
	in.RequiredFences = append([]eventstore.FenceRequest(nil), in.RequiredFences...)
	return Contract{&in}, nil
}

func (c Contract) same(other Contract) bool {
	// Fence declarations are local execution prerequisites, not source event or
	// bootstrap identity. The receiving adapter always requires its OWN complete
	// declaration before binding/effects; a weaker position cannot remove locks.
	if c.value == nil || other.value == nil {
		return false
	}
	a, b := c.value, other.value
	return a.LocalOwner == b.LocalOwner && a.SourceOwner == b.SourceOwner && a.Consumer == b.Consumer && a.AggregateType == b.AggregateType && a.AggregateID == b.AggregateID && a.Kind == b.Kind && a.Generation == b.Generation && a.ContractID == b.ContractID && a.ContractVersion == b.ContractVersion && a.ContractDigest == b.ContractDigest
}

// ValidatedPosition retains the exact original bytes' hash. NewPosition checks
// the owner-declared schema and permission even for an irrelevant event. The
// transaction adapter must independently revalidate current authorization.
type ValidatedPosition struct {
	contract   Contract
	envelope   events.Envelope
	hash       [32]byte
	irrelevant bool
}

func NewPosition[T any](contract Contract, raw []byte, schema events.EventSchema[T], irrelevant bool, authorize func(events.Envelope) error) (ValidatedPosition, error) {
	if contract.value == nil || authorize == nil {
		return ValidatedPosition{}, ErrInvalid
	}
	body := bytes.Clone(raw)
	var e events.Envelope
	if err := json.Unmarshal(body, &e); err != nil {
		return ValidatedPosition{}, ErrInvalid
	}
	if _, err := events.DecodeData(e, schema); err != nil {
		return ValidatedPosition{}, err
	}
	c := contract.value
	if e.Owner() != c.SourceOwner || e.AggregateType() != c.AggregateType || e.AggregateID() != c.AggregateID || !canonicalID(e.EventID()) {
		return ValidatedPosition{}, ErrInvalid
	}
	if err := authorize(e); err != nil {
		return ValidatedPosition{}, err
	}
	return ValidatedPosition{contract, e, sha256.Sum256(body), irrelevant}, nil
}

func (p ValidatedPosition) Envelope() events.Envelope { return p.envelope }
func (p ValidatedPosition) Hash() [32]byte            { return p.hash }

// BootstrapInput contains authenticated source evidence prepared outside SQL.
// A reference's shape is not authorization. VerifyBootstrap requires an owner
// verifier, and InstallBootstrap requires a separate current local recheck.
type BootstrapInput struct {
	BootstrapID, RequestID, AdmissionID                     string
	AuthorityRef, ScopeRef, PurposeRef, SourceCheckpointRef string
	StartAfter                                              int64
	SnapshotManifestRef                                     string
	SnapshotManifestHash                                    [32]byte
	NoSnapshotContractRef, NewEmptyProofRef                 string
}
type VerifiedBootstrap struct {
	contract Contract
	input    BootstrapInput
	valid    bool
}

func (b VerifiedBootstrap) Input() BootstrapInput { return b.input }

func VerifyBootstrap(ctx context.Context, c Contract, in BootstrapInput, verify func(context.Context, BootstrapInput) error) (VerifiedBootstrap, error) {
	if c.value == nil || verify == nil || !canonicalID(in.BootstrapID) || !canonicalID(in.RequestID) || (in.AdmissionID != "" && !canonicalID(in.AdmissionID)) || in.StartAfter < 0 {
		return VerifiedBootstrap{}, ErrInvalid
	}
	for _, ref := range []string{in.AuthorityRef, in.ScopeRef, in.PurposeRef, in.SourceCheckpointRef} {
		if !nonempty(ref) {
			return VerifiedBootstrap{}, ErrInvalid
		}
	}
	snapshot := nonempty(in.SnapshotManifestRef) && in.SnapshotManifestHash != ([32]byte{}) && in.NoSnapshotContractRef == ""
	noSnapshot := in.SnapshotManifestRef == "" && in.SnapshotManifestHash == ([32]byte{}) && nonempty(in.NoSnapshotContractRef)
	if (!snapshot && !noSnapshot) || (in.StartAfter == 0 && !nonempty(in.NewEmptyProofRef)) || (in.StartAfter > 0 && in.NewEmptyProofRef != "") {
		return VerifiedBootstrap{}, ErrInvalid
	}
	if err := verify(ctx, in); err != nil {
		return VerifiedBootstrap{}, err
	}
	return VerifiedBootstrap{c, in, true}, nil
}

// Gap is an error/candidate from the failed effect transaction, not persisted
// evidence. Return it from the outer Run (rolling inbox/effect/job back), then
// record it using a different transaction and fresh prepared fences.
type Gap struct {
	position  ValidatedPosition
	bootstrap string
	expected  int64
	xid       string
}

func (g *Gap) Error() string { return ErrGap.Error() }
func (g *Gap) Unwrap() error { return ErrGap }
func (g *Gap) MissingInterval() (int64, int64) {
	if g == nil {
		return 0, 0
	}
	return g.expected, g.position.envelope.IntegrationSequence().Int64() - 1
}

// RecoveryRequest can only be loaded from committed requested evidence with
// ReadRecovery. It contains no SQL handle, body or implicit process authority.
type RecoveryRequest struct {
	contract                    Contract
	gapID, attemptID, authority string
	from, through               int64
}

func (r RecoveryRequest) Interval() (int64, int64) { return r.from, r.through }
func (r RecoveryRequest) GapID() string            { return r.gapID }
func (r RecoveryRequest) AttemptID() string        { return r.attemptID }
func (r RecoveryRequest) AuthorityRef() string     { return r.authority }
func (r RecoveryRequest) Stream() (events.Owner, string, string) {
	if r.contract.value == nil {
		return "", "", ""
	}
	c := r.contract.value
	return c.SourceOwner, c.AggregateType, c.AggregateID
}

type HistoryBatch struct {
	Bodies       [][]byte
	ManifestRef  string
	ManifestHash [32]byte
}

// HistorySource authenticates exact original history and its purpose/scope.
// Fetch and Verify run outside SQL. There is deliberately no latest-state API.
type HistorySource interface {
	Fetch(context.Context, RecoveryRequest) (HistoryBatch, error)
	Verify(context.Context, RecoveryRequest, HistoryBatch) error
}
type VerifiedRecovery struct {
	request   RecoveryRequest
	batch     HistoryBatch
	positions []ValidatedPosition
}

// HistoryDigest binds an ordered manifest to the exact original bytes. The
// source verifier must also authenticate stream, interval and purpose; this
// digest alone is not a signature or permission.
func HistoryDigest(bodies [][]byte) [32]byte {
	h := sha256.New()
	h.Write([]byte("justixauto:original-history:v1"))
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(bodies)))
	h.Write(n[:])
	for _, b := range bodies {
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		h.Write(n[:])
		d := sha256.Sum256(b)
		h.Write(d[:])
	}
	var d [32]byte
	copy(d[:], h.Sum(nil))
	return d
}
func cloneBatch(b HistoryBatch) HistoryBatch {
	out := b
	out.Bodies = make([][]byte, len(b.Bodies))
	for i := range b.Bodies {
		out.Bodies[i] = bytes.Clone(b.Bodies[i])
	}
	return out
}

func FetchRecovery(ctx context.Context, r RecoveryRequest, source HistorySource, validate func([]byte) (ValidatedPosition, error)) (VerifiedRecovery, error) {
	if r.contract.value == nil || !canonicalID(r.gapID) || r.from <= 0 || r.through < r.from || source == nil || validate == nil {
		return VerifiedRecovery{}, ErrInvalid
	}
	b, err := source.Fetch(ctx, r)
	if err != nil {
		return VerifiedRecovery{}, err
	}
	b = cloneBatch(b)
	if int64(len(b.Bodies)) != r.through-r.from+1 || !nonempty(b.ManifestRef) || HistoryDigest(b.Bodies) != b.ManifestHash {
		return VerifiedRecovery{}, ErrConflict
	}
	if err = source.Verify(ctx, r, cloneBatch(b)); err != nil {
		return VerifiedRecovery{}, err
	}
	positions := make([]ValidatedPosition, len(b.Bodies))
	seen := map[string]bool{}
	for i, body := range b.Bodies {
		p, e := validate(bytes.Clone(body))
		if e != nil {
			return VerifiedRecovery{}, e
		}
		if !p.contract.same(r.contract) || p.hash != sha256.Sum256(body) || p.envelope.IntegrationSequence().Int64() != r.from+int64(i) || seen[p.envelope.EventID()] {
			return VerifiedRecovery{}, ErrConflict
		}
		seen[p.envelope.EventID()] = true
		positions[i] = p
	}
	return VerifiedRecovery{r, b, positions}, nil
}

type RecoveryPath uint8

const (
	DirectWithoutBrokerDelivery RecoveryPath = iota + 1
	CustodyIntake
)

// SubmitRecovery hands originals to the ordinary admission/inbox path. Direct
// submission has no broker delivery to ACK; custody means intake durability,
// not completion. Partial success is safe to resubmit through normal dedup.
// Recheck current authority in that ordinary path; never ACK the blocked event.
func SubmitRecovery(ctx context.Context, r VerifiedRecovery, path RecoveryPath, submit func(context.Context, RecoveryPath, []byte) error) error {
	if r.request.contract.value == nil || len(r.positions) == 0 || submit == nil || (path != DirectWithoutBrokerDelivery && path != CustodyIntake) {
		return ErrInvalid
	}
	for _, body := range r.batch.Bodies {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := submit(ctx, path, bytes.Clone(body)); err != nil {
			return err
		}
	}
	return nil
}
func (r VerifiedRecovery) Manifest() (string, [32]byte) {
	return r.batch.ManifestRef, r.batch.ManifestHash
}
