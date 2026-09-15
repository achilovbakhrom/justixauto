package eventstore

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"justixauto/pkg/events"
)

var ErrFencePreparation = errors.New("invalid or incomplete receiver fence preparation")

type FenceMode uint8

const (
	SharedFence FenceMode = iota + 1
	ExclusiveFence
)

// FenceRequest is an immutable mechanical lock declaration, not authority to
// access a stream, generation or business guard. Only constructors create it.
type FenceRequest struct {
	key   string
	mode  FenceMode
	phase uint8
	owner events.Owner
}

var fenceName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func canonicalFenceUUID(s string) bool {
	id, err := uuid.Parse(s)
	return err == nil && id != uuid.Nil && id.String() == s
}
func fenceComponent(s string) bool {
	return s != "" && strings.TrimSpace(s) == s && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
func validFenceMode(m FenceMode) bool { return m == SharedFence || m == ExclusiveFence }

// SourceFence declares an owner-local T-009 append prerequisite. PrepareFences
// acquires these first using exactly T-009's key; an effect must not discover
// more source streams after preparation. This does not append or authorize.
func SourceFence(local events.Owner, aggregateType, aggregateID string) (FenceRequest, error) {
	if !local.Valid() || !fenceName.MatchString(aggregateType) || !canonicalFenceUUID(aggregateID) {
		return FenceRequest{}, ErrFencePreparation
	}
	return FenceRequest{"justixauto:eventstore:" + string(local) + ":" + aggregateType + ":" + aggregateID, ExclusiveFence, 1, local}, nil
}

func ReceiverFence(local, source events.Owner, aggregateType, aggregateID string) (FenceRequest, error) {
	if !local.Valid() || !source.Valid() || !fenceName.MatchString(aggregateType) || !canonicalFenceUUID(aggregateID) {
		return FenceRequest{}, ErrFencePreparation
	}
	return FenceRequest{"justixauto:receiver:" + string(local) + ":" + string(source) + ":" + aggregateType + ":" + aggregateID, ExclusiveFence, 3, local}, nil
}

func encodedFence(namespace string, components ...string) string {
	var b strings.Builder
	b.WriteString(namespace)
	for _, c := range components {
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(len(c)))
		b.WriteByte(':')
		b.WriteString(c)
	}
	return b.String()
}

func GenerationFence(local events.Owner, projection, generation string, mode FenceMode) (FenceRequest, error) {
	if !local.Valid() || !fenceComponent(projection) || !canonicalFenceUUID(generation) || !validFenceMode(mode) {
		return FenceRequest{}, ErrFencePreparation
	}
	return FenceRequest{encodedFence("justixauto:projection-generation", string(local), projection, generation), mode, 4, local}, nil
}

// GuardFence requires the approved owner-defined contract and canonical scope
// mapping. This helper checks encoding shape, not the meaning of that mapping.
func GuardFence(local events.Owner, contract, scope string, mode FenceMode) (FenceRequest, error) {
	if !local.Valid() || !fenceComponent(contract) || !fenceComponent(scope) || !validFenceMode(mode) {
		return FenceRequest{}, ErrFencePreparation
	}
	return FenceRequest{encodedFence("justixauto:command-guard", string(local), contract, scope), mode, 4, local}, nil
}

// PreparedFences is sealed to the actual outer transaction and its complete
// declared lock set. Copies share immutable state. It exposes no SQL/commit API.
type PreparedFences struct{ state *preparedFenceState }
type preparedFenceState struct {
	owner      events.Owner
	token, xid string
	backend    int
	catalog    FenceMode
	keys       map[string]FenceMode
}

func fenceTransaction(ctx context.Context, tx *gorm.DB) (string, int, string, error) {
	if err := requireTransaction(tx); err != nil {
		return "", 0, "", err
	}
	var r struct {
		Isolation, Xid, Token string
		Backend               int
	}
	err := tx.WithContext(ctx).Raw(`SELECT current_setting('transaction_isolation') AS isolation,
	 pg_current_xact_id()::text AS xid, pg_backend_pid() AS backend,
	 coalesce(current_setting('justix.prepared_receiver_fences',true),'') AS token`).Scan(&r).Error
	if err != nil {
		return "", 0, "", err
	}
	if r.Isolation != "read committed" || r.Xid == "" || r.Backend == 0 {
		return "", 0, "", ErrFencePreparation
	}
	return r.Xid, r.Backend, r.Token, nil
}

// PrepareFences acquires one complete set exactly once in this transaction:
// sorted source prerequisites, catalog, sorted receivers, sorted generation/
// guard keys. Repeated keys merge to exclusive before acquisition. Every error
// must abort the outer transaction; no lock upgrades or automatic retries occur.
// Call before head/job/inbox/checkpoint/effect rows. Cooperating owner adapters
// must declare all required keys and call Require before effects. Advisory locks
// cannot discover uncooperative business writers or attest authorization.
func PrepareFences(ctx context.Context, tx *gorm.DB, owner events.Owner, catalog FenceMode, requests ...FenceRequest) (PreparedFences, error) {
	if !owner.Valid() || !validFenceMode(catalog) {
		return PreparedFences{}, ErrFencePreparation
	}
	keys := make(map[string]FenceRequest, len(requests))
	for _, r := range requests {
		if r.owner != owner || r.key == "" || !validFenceMode(r.mode) || (r.phase != 1 && r.phase != 3 && r.phase != 4) {
			return PreparedFences{}, ErrFencePreparation
		}
		if old, ok := keys[r.key]; ok && old.mode > r.mode {
			r.mode = old.mode
		}
		keys[r.key] = r
	}
	xid, backend, existing, err := fenceTransaction(ctx, tx)
	if err != nil {
		return PreparedFences{}, err
	}
	if existing != "" {
		return PreparedFences{}, ErrFencePreparation
	}
	token := uuid.NewString()
	if err := tx.WithContext(ctx).Exec("SELECT set_config('justix.prepared_receiver_fences',?,true)", token).Error; err != nil {
		return PreparedFences{}, err
	}
	ordered := make([]FenceRequest, 0, len(keys)+1)
	for _, r := range keys {
		ordered = append(ordered, r)
	}
	ordered = append(ordered, FenceRequest{"justixauto:receiver-catalog:" + string(owner), catalog, 2, owner})
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].phase != ordered[j].phase {
			return ordered[i].phase < ordered[j].phase
		}
		return ordered[i].key < ordered[j].key
	})
	for _, r := range ordered {
		query := "SELECT pg_advisory_xact_lock(hashtextextended(?,0))"
		if r.mode == SharedFence {
			query = "SELECT pg_advisory_xact_lock_shared(hashtextextended(?,0))"
		}
		if err := tx.WithContext(ctx).Exec(query, r.key).Error; err != nil {
			return PreparedFences{}, err
		}
	}
	// Recheck the actual server transaction after acquisition. A pool wrapper
	// that merely implements TxCommitter cannot turn statement-scoped locks
	// into a transaction capability: its xid/local token will not survive.
	finalXID, finalBackend, finalToken, err := fenceTransaction(ctx, tx)
	if err != nil {
		return PreparedFences{}, err
	}
	if finalXID != xid || finalBackend != backend || finalToken != token {
		return PreparedFences{}, ErrFencePreparation
	}
	covered := make(map[string]FenceMode, len(keys))
	for k, r := range keys {
		covered[k] = r.mode
	}
	return PreparedFences{&preparedFenceState{owner, token, xid, backend, catalog, covered}}, nil
}

// Require rejects absent/wrong-mode keys and a foreign, ended or unprepared
// transaction before an adapter reads/writes effects. Cloned GORM sessions of
// the same SQL transaction are accepted; a fresh pool/transaction is not.
func (p PreparedFences) Require(ctx context.Context, tx *gorm.DB, owner events.Owner, catalog FenceMode, requests ...FenceRequest) error {
	s := p.state
	if s == nil || s.owner != owner || !validFenceMode(catalog) || s.catalog < catalog {
		return ErrFencePreparation
	}
	if err := requireTransaction(tx); err != nil {
		return err
	}
	for _, r := range requests {
		if r.owner != owner || r.key == "" || !validFenceMode(r.mode) || s.keys[r.key] < r.mode {
			return fmt.Errorf("%w: required key or mode absent", ErrFencePreparation)
		}
	}
	xid, backend, token, err := fenceTransaction(ctx, tx)
	if err != nil {
		return err
	}
	if xid != s.xid || backend != s.backend || token != s.token {
		return ErrFencePreparation
	}
	return nil
}
