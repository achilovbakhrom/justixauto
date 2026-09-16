package inbox

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"justixauto/pkg/events"
)

const MembershipFormat = "membership-v1"

var (
	ErrMembershipIdentity = errors.New("invalid membership identity")
	ErrMembershipConflict = errors.New("conflicting retained membership identity")
	ErrMembershipLimit    = errors.New("membership resource ceiling exceeded or unspecified")
)

// MembershipLimits are explicit trusted resource ceilings, not business policy.
// MaxValues counts JSON values and object keys; MaxDepth counts JSON containers.
// There are no implicit defaults. Exceeding a ceiling never truncates a set.
type MembershipLimits struct{ MaxBytes, MaxValues, MaxDepth int }

// MembershipDigest uses exactly 64 lowercase hexadecimal characters on the wire.
type MembershipDigest [32]byte

func (d MembershipDigest) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(d[:]))
}
func (d *MembershipDigest) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return ErrMembershipIdentity
	}
	if len(s) != 64 || strings.ToLower(s) != s {
		return ErrMembershipIdentity
	}
	v, err := hex.DecodeString(s)
	if err != nil {
		return ErrMembershipIdentity
	}
	copy(d[:], v)
	return nil
}

type MembershipEvidence struct {
	Reference string           `json:"reference"`
	Digest    MembershipDigest `json:"digest"`
}
type MembershipContract struct {
	ID      string           `json:"id"`
	Version uint32           `json:"version"`
	Digest  MembershipDigest `json:"digest"`
}
type MembershipSchema struct {
	EventType string `json:"event_type"`
	Version   uint32 `json:"version"`
}
type MembershipFeature struct {
	ID       string             `json:"id"`
	Requires []string           `json:"requires"`
	Contract MembershipContract `json:"contract"`
}

// ConsumerMeaning excludes approved exchange/queue placement. Moving a binding
// does not change this immutable meaning or permit a checkpoint reset.
type ConsumerMeaning struct {
	Format          string             `json:"format"`
	Owner           events.Owner       `json:"owner"`
	FeatureID       string             `json:"feature_id"`
	Name            string             `json:"name"`
	Kind            string             `json:"kind"`
	Generation      string             `json:"generation"`
	Subscription    MembershipContract `json:"subscription"`
	SourceOwner     events.Owner       `json:"source_owner"`
	StreamID        string             `json:"stream_id"`
	Selector        MembershipContract `json:"selector"`
	SchemaSemantics MembershipEvidence `json:"schema_semantics"`
	RoutingKeys     []string           `json:"routing_keys"`
	Schemas         []MembershipSchema `json:"schemas"`
}
type ConsumerBinding struct {
	Exchange string             `json:"exchange"`
	Queue    string             `json:"queue"`
	Approval MembershipEvidence `json:"approval"`
}
type CatalogClaim struct {
	Format  string          `json:"format"`
	Meaning ConsumerMeaning `json:"meaning"`
	Binding ConsumerBinding `json:"binding"`
}
type CatalogSpec struct {
	Format          string              `json:"format"`
	Owner           events.Owner        `json:"owner"`
	Version         string              `json:"version"`
	Approval        MembershipEvidence  `json:"approval"`
	BindingManifest MembershipEvidence  `json:"binding_manifest"`
	Features        []MembershipFeature `json:"features"`
	Claims          []CatalogClaim      `json:"claims"`
}

// These DTOs mirror T-746's inert fields without implementing its registries or
// importing an owner's packages. Root code must compare its complete frozen
// projection/process claims before Bind; no factory output supplies metadata.
type FrozenSubscriptionClaim struct {
	ContractID                string
	SourceOwner               events.Owner
	StreamID, Exchange, Queue string
	RoutingKeys               []string
	Schemas                   []MembershipSchema
}
type FrozenConsumerClaim struct {
	Name, Kind   string
	Subscription FrozenSubscriptionClaim
}
type FrozenFeatureClaims struct {
	Owner     events.Owner
	ID        string
	Requires  []string
	Consumers []FrozenConsumerClaim
}
type MembershipSupplement struct {
	Meaning ConsumerMeaning
	Binding ConsumerBinding
}

type SelectedConsumer struct {
	Name        string             `json:"name"`
	ClaimDigest MembershipDigest   `json:"claim_digest"`
	State       string             `json:"state"`
	Disposition MembershipEvidence `json:"disposition"`
}
type SelectionSpec struct {
	Format         string             `json:"format"`
	Owner          events.Owner       `json:"owner"`
	CatalogVersion string             `json:"catalog_version"`
	CatalogDigest  MembershipDigest   `json:"catalog_digest"`
	Approval       MembershipEvidence `json:"approval"`
	Consumers      []SelectedConsumer `json:"consumers"`
}
type MembershipStream struct {
	SourceOwner   events.Owner `json:"source_owner"`
	AggregateType string       `json:"aggregate_type"`
	AggregateID   string       `json:"aggregate_id"`
}
type UniverseMember struct {
	Stream             MembershipStream     `json:"stream"`
	Selectors          []MembershipContract `json:"selectors"`
	SourceVerification MembershipEvidence   `json:"source_verification"`
	Disposition        MembershipEvidence   `json:"disposition"`
}
type StreamUniverseSpec struct {
	Format    string             `json:"format"`
	Owner     events.Owner       `json:"owner"`
	Authority MembershipEvidence `json:"authority"`
	Streams   []UniverseMember   `json:"streams"`
}

// BootstrapIdentity records the existing source-issued bootstrap identity; it
// is not a VerifiedBootstrap or authority to install a snapshot/process effect.
type BootstrapIdentity struct {
	ID                    string              `json:"id"`
	RequestID             string              `json:"request_id"`
	AdmissionID           string              `json:"admission_id"`
	StartAfter            int64               `json:"start_after"`
	AuthorityRef          string              `json:"authority_ref"`
	ScopeRef              string              `json:"scope_ref"`
	PurposeRef            string              `json:"purpose_ref"`
	SourceCheckpointRef   string              `json:"source_checkpoint_ref"`
	Snapshot              *MembershipEvidence `json:"snapshot"`
	NoSnapshotContractRef *string             `json:"no_snapshot_contract_ref"`
	NewEmptyProofRef      *string             `json:"new_empty_proof_ref"`
}
type StreamConsumer struct {
	Name                   string              `json:"name"`
	ClaimDigest            MembershipDigest    `json:"claim_digest"`
	Kind                   string              `json:"kind"`
	Generation             string              `json:"generation"`
	AdmissionRootID        string              `json:"admission_root_id"`
	AdmissionTipID         string              `json:"admission_tip_id"`
	ExpectedAdmissionTipID *string             `json:"expected_admission_tip_id"`
	StartAfter             int64               `json:"start_after"`
	EndInclusive           *int64              `json:"end_inclusive"`
	Bootstrap              BootstrapIdentity   `json:"bootstrap"`
	ProcessCatchUp         *MembershipEvidence `json:"process_catch_up"`
	Disposition            MembershipEvidence  `json:"disposition"`
	EffectDigest           MembershipDigest    `json:"effect_digest"`
}
type ConsumerExclusion struct {
	Name        string             `json:"name"`
	ClaimDigest MembershipDigest   `json:"claim_digest"`
	Authority   MembershipEvidence `json:"authority"`
}
type EnrollmentConsumer struct {
	Name        string `json:"name"`
	AdmissionID string `json:"admission_id"`
}
type EnrollmentEffect struct {
	EventID        string               `json:"event_id"`
	EnrollmentID   string               `json:"enrollment_id"`
	Position       int64                `json:"position"`
	EnvelopeDigest MembershipDigest     `json:"envelope_digest"`
	Phase          string               `json:"phase"`
	Consumers      []EnrollmentConsumer `json:"consumers"`
	EffectDigest   MembershipDigest     `json:"effect_digest"`
}
type StreamSelectionIdentity struct {
	RequestID string           `json:"request_id"`
	Digest    MembershipDigest `json:"digest"`
}
type StreamSelectionSpec struct {
	Format           string                   `json:"format"`
	Owner            events.Owner             `json:"owner"`
	Stream           MembershipStream         `json:"stream"`
	CatalogVersion   string                   `json:"catalog_version"`
	CatalogDigest    MembershipDigest         `json:"catalog_digest"`
	SelectionDigest  MembershipDigest         `json:"selection_digest"`
	UniverseDigest   MembershipDigest         `json:"universe_digest"`
	ExpectedPrevious *StreamSelectionIdentity `json:"expected_previous"`
	Authority        MembershipEvidence       `json:"authority"`
	SourceProof      MembershipEvidence       `json:"source_proof"`
	Boundary         int64                    `json:"boundary"`
	ZeroApplicable   *MembershipEvidence      `json:"zero_applicable"`
	Consumers        []StreamConsumer         `json:"consumers"`
	Exclusions       []ConsumerExclusion      `json:"exclusions"`
	Backlog          []EnrollmentEffect       `json:"backlog"`
}
type MembershipHeadIdentity struct {
	Epoch           int64            `json:"epoch"`
	RequestID       string           `json:"request_id"`
	CatalogVersion  string           `json:"catalog_version"`
	CatalogDigest   MembershipDigest `json:"catalog_digest"`
	SelectionDigest MembershipDigest `json:"selection_digest"`
}
type TransitionRequestSpec struct {
	Format        string                  `json:"format"`
	Owner         events.Owner            `json:"owner"`
	RequestID     string                  `json:"request_id"`
	Action        string                  `json:"action"`
	ExpectedHead  *MembershipHeadIdentity `json:"expected_head"`
	ResultEpoch   int64                   `json:"result_epoch"`
	Approval      MembershipEvidence      `json:"approval"`
	Catalog       CatalogSpec             `json:"catalog"`
	Selection     SelectionSpec           `json:"selection"`
	Universe      StreamUniverseSpec      `json:"universe"`
	Streams       []StreamSelectionSpec   `json:"streams"`
	EffectsDigest MembershipDigest        `json:"effects_digest"`
}

// These are sealed identity values only. No value proves source authority,
// completeness of external discovery, current storage, commit or broker ACK.
type membershipValue[T any] struct {
	canonical []byte
	digest    MembershipDigest
}

func (v membershipValue[T]) Valid() bool              { return len(v.canonical) > 0 }
func (v membershipValue[T]) Bytes() []byte            { return bytes.Clone(v.canonical) }
func (v membershipValue[T]) Digest() MembershipDigest { return v.digest }
func (v membershipValue[T]) Specification() T {
	var out T
	if v.Valid() {
		_ = json.Unmarshal(v.canonical, &out)
	}
	return out
}

type Catalog struct{ membershipValue[CatalogSpec] }
type Selection struct{ membershipValue[SelectionSpec] }
type StreamUniverse struct {
	membershipValue[StreamUniverseSpec]
}
type StreamSelection struct {
	membershipValue[StreamSelectionSpec]
}
type TransitionRequest struct {
	membershipValue[TransitionRequestSpec]
}

func NewCatalog(in CatalogSpec, limits MembershipLimits) (Catalog, error) {
	s, err := membershipCopy(in, limits)
	if err != nil {
		return Catalog{}, err
	}
	if s.Format != MembershipFormat || !s.Owner.Valid() || !membershipLabel(s.Version) || !validEvidence(s.Approval) || !validEvidence(s.BindingManifest) {
		return Catalog{}, ErrMembershipIdentity
	}
	if err := sortMembershipSet(s.Features, func(a, b MembershipFeature) int { return strings.Compare(a.ID, b.ID) }); err != nil {
		return Catalog{}, err
	}
	features := map[string]MembershipFeature{}
	for i := range s.Features {
		f := &s.Features[i]
		if !featureName(s.Owner, f.ID) || !validContract(f.Contract) {
			return Catalog{}, ErrMembershipIdentity
		}
		if err := sortMembershipSet(f.Requires, strings.Compare); err != nil {
			return Catalog{}, err
		}
		features[f.ID] = *f
	}
	// Exact local dependency existence and acyclicity; no registry or Bind exists.
	visited := map[string]uint8{}
	var visit func(string) bool
	visit = func(id string) bool {
		if visited[id] == 1 {
			return false
		}
		if visited[id] == 2 {
			return true
		}
		f, ok := features[id]
		if !ok {
			return false
		}
		visited[id] = 1
		for _, dep := range f.Requires {
			if !visit(dep) {
				return false
			}
		}
		visited[id] = 2
		return true
	}
	for _, f := range s.Features {
		if !visit(f.ID) {
			return Catalog{}, ErrMembershipIdentity
		}
	}
	if err := sortMembershipSet(s.Claims, func(a, b CatalogClaim) int { return strings.Compare(a.Meaning.Name, b.Meaning.Name) }); err != nil {
		return Catalog{}, err
	}
	for i := range s.Claims {
		c := &s.Claims[i]
		m := &c.Meaning
		if c.Format != MembershipFormat || m.Format != MembershipFormat || m.Owner != s.Owner || !membershipText(m.Name) || !validKind(m.Kind) || !membershipLabel(m.Generation) || !validContract(m.Subscription) || !m.SourceOwner.Valid() || !membershipText(m.StreamID) || !validContract(m.Selector) || !validEvidence(m.SchemaSemantics) || !membershipText(c.Binding.Exchange) || !membershipText(c.Binding.Queue) || !validEvidence(c.Binding.Approval) {
			return Catalog{}, ErrMembershipIdentity
		}
		if _, ok := features[m.FeatureID]; !ok {
			return Catalog{}, ErrMembershipIdentity
		}
		if len(m.RoutingKeys) == 0 || len(m.Schemas) == 0 {
			return Catalog{}, ErrMembershipIdentity
		}
		if err := sortMembershipSet(m.RoutingKeys, strings.Compare); err != nil {
			return Catalog{}, err
		}
		for _, key := range m.RoutingKeys {
			if !membershipText(key) {
				return Catalog{}, ErrMembershipIdentity
			}
		}
		if err := sortMembershipSet(m.Schemas, compareSchema); err != nil {
			return Catalog{}, err
		}
		for _, schema := range m.Schemas {
			if !validMembershipSchema(m.SourceOwner, schema) {
				return Catalog{}, ErrMembershipIdentity
			}
		}
	}
	v, err := sealMembership(s, limits)
	return Catalog{v}, err
}

// MatchClaims compares complete inert DTO sets and every T-746 overlap. Root
// wiring must call this before any Bind. It does not implement registration or
// authenticate the additional release-supplied contract digests and generation.
func (c Catalog) MatchClaims(frozen []FrozenFeatureClaims, supplements []MembershipSupplement, limits MembershipLimits) error {
	if !c.Valid() {
		return ErrMembershipIdentity
	}
	f, err := membershipCopy(frozen, limits)
	if err != nil {
		return err
	}
	supp, err := membershipCopy(supplements, limits)
	if err != nil {
		return err
	}
	s := c.Specification()
	features := map[string]MembershipFeature{}
	for _, v := range s.Features {
		features[v.ID] = v
	}
	claims := map[string]CatalogClaim{}
	for _, v := range s.Claims {
		claims[v.Meaning.Name] = v
	}
	bySupplement := map[string]MembershipSupplement{}
	for _, v := range supp {
		m := v.Meaning
		if m.Owner != s.Owner {
			return ErrMembershipIdentity
		}
		if _, ok := bySupplement[m.Name]; ok {
			return ErrMembershipIdentity
		}
		bySupplement[m.Name] = v
	}
	seenFeature, seenConsumer := map[string]bool{}, map[string]bool{}
	for _, feature := range f {
		installed, ok := features[feature.ID]
		if !ok || feature.Owner != s.Owner || seenFeature[feature.ID] {
			return ErrMembershipIdentity
		}
		seenFeature[feature.ID] = true
		if err := sortMembershipSet(feature.Requires, strings.Compare); err != nil {
			return err
		}
		if !slices.Equal(feature.Requires, installed.Requires) {
			return ErrMembershipIdentity
		}
		for _, claim := range feature.Consumers {
			if seenConsumer[claim.Name] {
				return ErrMembershipIdentity
			}
			seenConsumer[claim.Name] = true
			catalogClaim, ok := claims[claim.Name]
			extra, found := bySupplement[claim.Name]
			if !ok || !found {
				return ErrMembershipIdentity
			}
			m := extra.Meaning
			b := extra.Binding
			if m.Owner != feature.Owner || m.FeatureID != feature.ID || m.Name != claim.Name || m.Kind != claim.Kind || m.Subscription.ID != claim.Subscription.ContractID || m.SourceOwner != claim.Subscription.SourceOwner || m.StreamID != claim.Subscription.StreamID || b.Exchange != claim.Subscription.Exchange || b.Queue != claim.Subscription.Queue {
				return ErrMembershipIdentity
			}
			if err := sortMembershipSet(claim.Subscription.RoutingKeys, strings.Compare); err != nil {
				return err
			}
			if err := sortMembershipSet(claim.Subscription.Schemas, compareSchema); err != nil {
				return err
			}
			if err := sortMembershipSet(m.RoutingKeys, strings.Compare); err != nil {
				return err
			}
			if err := sortMembershipSet(m.Schemas, compareSchema); err != nil {
				return err
			}
			if !slices.Equal(m.RoutingKeys, claim.Subscription.RoutingKeys) || !slices.Equal(m.Schemas, claim.Subscription.Schemas) {
				return ErrMembershipIdentity
			}
			if !reflect.DeepEqual(CatalogClaim{Format: MembershipFormat, Meaning: m, Binding: b}, catalogClaim) {
				return ErrMembershipIdentity
			}
		}
	}
	if len(seenFeature) != len(features) || len(seenConsumer) != len(claims) || len(bySupplement) != len(claims) {
		return ErrMembershipIdentity
	}
	return nil
}

func (c Catalog) Claim(name string) (CatalogClaim, bool) {
	if !c.Valid() {
		return CatalogClaim{}, false
	}
	for _, claim := range c.Specification().Claims {
		if claim.Meaning.Name == name {
			return claim, true
		}
	}
	return CatalogClaim{}, false
}
func (c Catalog) ClaimBytes(name string) ([]byte, MembershipDigest, error) {
	v, ok := c.Claim(name)
	if !ok {
		return nil, MembershipDigest{}, ErrMembershipIdentity
	}
	b := membershipJSON(v)
	return b, MembershipDigest(sha256.Sum256(b)), nil
}
func (c Catalog) MeaningBytes(name string) ([]byte, MembershipDigest, error) {
	v, ok := c.Claim(name)
	if !ok {
		return nil, MembershipDigest{}, ErrMembershipIdentity
	}
	b := membershipJSON(v.Meaning)
	return b, MembershipDigest(sha256.Sum256(b)), nil
}

// CheckCatalogIdentity compares only the supplied retained/new values. T-948
// must compare all relevant retained history; this helper never queries it.
func CheckCatalogIdentity(retained, next Catalog) error {
	if !retained.Valid() || !next.Valid() {
		return ErrMembershipIdentity
	}
	a, b := retained.Specification(), next.Specification()
	if a.Owner != b.Owner {
		return ErrMembershipIdentity
	}
	if a.Version == b.Version && !bytes.Equal(retained.canonical, next.canonical) {
		return ErrMembershipConflict
	}
	old := map[string]ConsumerMeaning{}
	for _, c := range a.Claims {
		old[c.Meaning.Name] = c.Meaning
	}
	for _, c := range b.Claims {
		if meaning, ok := old[c.Meaning.Name]; ok && !reflect.DeepEqual(meaning, c.Meaning) {
			return ErrMembershipConflict
		}
	}
	return nil
}

func NewSelection(in SelectionSpec, catalog Catalog, limits MembershipLimits) (Selection, error) {
	if !catalog.Valid() {
		return Selection{}, ErrMembershipIdentity
	}
	s, err := membershipCopy(in, limits)
	if err != nil {
		return Selection{}, err
	}
	c := catalog.Specification()
	if s.Format != MembershipFormat || s.Owner != c.Owner || s.CatalogVersion != c.Version || s.CatalogDigest != catalog.Digest() || !validEvidence(s.Approval) {
		return Selection{}, ErrMembershipIdentity
	}
	if err := sortMembershipSet(s.Consumers, func(a, b SelectedConsumer) int { return strings.Compare(a.Name, b.Name) }); err != nil {
		return Selection{}, err
	}
	hashes := map[string]MembershipDigest{}
	for _, claim := range c.Claims {
		hashes[claim.Meaning.Name] = MembershipDigest(sha256.Sum256(membershipJSON(claim)))
	}
	for _, v := range s.Consumers {
		hash, ok := hashes[v.Name]
		if !ok || v.ClaimDigest != hash || !slices.Contains([]string{"active", "draining", "held"}, v.State) || !validEvidence(v.Disposition) {
			return Selection{}, ErrMembershipIdentity
		}
	}
	v, err := sealMembership(s, limits)
	return Selection{v}, err
}

func NewStreamUniverse(in StreamUniverseSpec, limits MembershipLimits) (StreamUniverse, error) {
	s, err := membershipCopy(in, limits)
	if err != nil {
		return StreamUniverse{}, err
	}
	if s.Format != MembershipFormat || !s.Owner.Valid() || !validEvidence(s.Authority) {
		return StreamUniverse{}, ErrMembershipIdentity
	}
	if err := sortMembershipSet(s.Streams, func(a, b UniverseMember) int { return compareStream(a.Stream, b.Stream) }); err != nil {
		return StreamUniverse{}, err
	}
	for i := range s.Streams {
		v := &s.Streams[i]
		if !validStream(v.Stream) || len(v.Selectors) == 0 || !validEvidence(v.SourceVerification) || !validEvidence(v.Disposition) {
			return StreamUniverse{}, ErrMembershipIdentity
		}
		if err := sortMembershipSet(v.Selectors, compareMembershipContract); err != nil {
			return StreamUniverse{}, err
		}
		for _, selector := range v.Selectors {
			if !validContract(selector) {
				return StreamUniverse{}, ErrMembershipIdentity
			}
		}
	}
	v, err := sealMembership(s, limits)
	return StreamUniverse{v}, err
}

func NewStreamSelection(in StreamSelectionSpec, catalog Catalog, selection Selection, universe StreamUniverse, limits MembershipLimits) (StreamSelection, error) {
	if !catalog.Valid() || !selection.Valid() || !universe.Valid() {
		return StreamSelection{}, ErrMembershipIdentity
	}
	if err := measureMembership(reflect.ValueOf(in), limits); err != nil {
		return StreamSelection{}, err
	}
	return newStreamSelection(in, streamIdentityContext(catalog, selection, universe), limits)
}

// This is a temporary read-only index over already sealed identity values,
// never a CurrentSelection/ProspectiveSelection transaction capability.
type membershipStreamIdentity struct {
	c          CatalogSpec
	o          SelectionSpec
	u          StreamUniverseSpec
	cd, sd, ud MembershipDigest
	claims     map[string]CatalogClaim
	selected   map[string]SelectedConsumer
	members    map[MembershipStream]UniverseMember
}

func streamIdentityContext(c Catalog, o Selection, u StreamUniverse) membershipStreamIdentity {
	x := membershipStreamIdentity{c: c.Specification(), o: o.Specification(), u: u.Specification(), cd: c.Digest(), sd: o.Digest(), ud: u.Digest(), claims: map[string]CatalogClaim{}, selected: map[string]SelectedConsumer{}, members: map[MembershipStream]UniverseMember{}}
	for _, v := range x.c.Claims {
		x.claims[v.Meaning.Name] = v
	}
	for _, v := range x.o.Consumers {
		x.selected[v.Name] = v
	}
	for _, v := range x.u.Streams {
		x.members[v.Stream] = v
	}
	return x
}
func newStreamSelection(in StreamSelectionSpec, x membershipStreamIdentity, limits MembershipLimits) (StreamSelection, error) {
	s, err := membershipCopy(in, limits)
	if err != nil {
		return StreamSelection{}, err
	}
	c, o, u := x.c, x.o, x.u
	if s.Format != MembershipFormat || s.Owner != c.Owner || s.Owner != o.Owner || s.Owner != u.Owner || s.CatalogVersion != c.Version || s.CatalogDigest != x.cd || s.SelectionDigest != x.sd || s.UniverseDigest != x.ud || o.CatalogDigest != x.cd || s.Boundary < 0 || !validStream(s.Stream) || !validEvidence(s.Authority) || !validEvidence(s.SourceProof) {
		return StreamSelection{}, ErrMembershipIdentity
	}
	member, exists := x.members[s.Stream]
	if !exists {
		return StreamSelection{}, ErrMembershipIdentity
	}
	if s.ExpectedPrevious != nil && (!membershipUUID(s.ExpectedPrevious.RequestID) || zeroDigest(s.ExpectedPrevious.Digest)) {
		return StreamSelection{}, ErrMembershipIdentity
	}
	if (len(s.Consumers) == 0) != (s.ZeroApplicable != nil) || (s.ZeroApplicable != nil && !validEvidence(*s.ZeroApplicable)) {
		return StreamSelection{}, ErrMembershipIdentity
	}
	if err := sortMembershipSet(s.Consumers, func(a, b StreamConsumer) int { return strings.Compare(a.Name, b.Name) }); err != nil {
		return StreamSelection{}, err
	}
	if err := sortMembershipSet(s.Exclusions, func(a, b ConsumerExclusion) int { return strings.Compare(a.Name, b.Name) }); err != nil {
		return StreamSelection{}, err
	}
	selected := x.selected
	accounted := map[string]bool{}
	consumers := map[string]StreamConsumer{}
	for _, v := range s.Consumers {
		claim, ok := x.claims[v.Name]
		selectedClaim, active := selected[v.Name]
		if !ok || !active || v.ClaimDigest != selectedClaim.ClaimDigest || v.Kind != claim.Meaning.Kind || v.Generation != claim.Meaning.Generation || claim.Meaning.SourceOwner != s.Stream.SourceOwner || !slices.Contains(member.Selectors, claim.Meaning.Selector) || !membershipUUID(v.AdmissionRootID) || !membershipUUID(v.AdmissionTipID) || (v.ExpectedAdmissionTipID != nil && !membershipUUID(*v.ExpectedAdmissionTipID)) || v.StartAfter < 0 || (v.EndInclusive != nil && *v.EndInclusive < v.StartAfter) || !validBootstrap(v.Bootstrap) || v.Bootstrap.AdmissionID != v.AdmissionRootID || v.Bootstrap.StartAfter != v.StartAfter || v.StartAfter > s.Boundary || !validEvidence(v.Disposition) || zeroDigest(v.EffectDigest) {
			return StreamSelection{}, ErrMembershipIdentity
		}
		if v.Kind == "process" && (v.ProcessCatchUp == nil || !validEvidence(*v.ProcessCatchUp)) {
			return StreamSelection{}, ErrMembershipIdentity
		}
		if v.ProcessCatchUp != nil && !validEvidence(*v.ProcessCatchUp) {
			return StreamSelection{}, ErrMembershipIdentity
		}
		accounted[v.Name] = true
		consumers[v.Name] = v
	}
	for _, v := range s.Exclusions {
		expected, ok := selected[v.Name]
		if !ok || accounted[v.Name] || v.ClaimDigest != expected.ClaimDigest || !validEvidence(v.Authority) {
			return StreamSelection{}, ErrMembershipIdentity
		}
		accounted[v.Name] = true
	}
	if len(accounted) != len(selected) {
		return StreamSelection{}, ErrMembershipIdentity
	}
	if err := sortMembershipSet(s.Backlog, func(a, b EnrollmentEffect) int {
		if x := strings.Compare(a.EventID, b.EventID); x != 0 {
			return x
		}
		return strings.Compare(a.EnrollmentID, b.EnrollmentID)
	}); err != nil {
		return StreamSelection{}, err
	}
	positions := map[int64]string{}
	eventPositions := map[string]int64{}
	eventHashes := map[string]MembershipDigest{}
	jobs := map[[2]string]bool{}
	enrollments := map[string]bool{}
	initialEvents := map[string]bool{}
	for i := range s.Backlog {
		v := &s.Backlog[i]
		if !membershipUUID(v.EventID) || !membershipUUID(v.EnrollmentID) || v.Position <= 0 || v.Position > s.Boundary || zeroDigest(v.EnvelopeDigest) || zeroDigest(v.EffectDigest) || (v.Phase != "initial" && v.Phase != "late") || (v.Phase == "late" && len(v.Consumers) == 0) || enrollments[v.EnrollmentID] {
			return StreamSelection{}, ErrMembershipIdentity
		}
		enrollments[v.EnrollmentID] = true
		// An empty recipient set still has one initial enrollment identity.
		if v.Phase == "initial" {
			if initialEvents[v.EventID] {
				return StreamSelection{}, ErrMembershipIdentity
			}
			initialEvents[v.EventID] = true
		}
		if event, ok := positions[v.Position]; ok && event != v.EventID {
			return StreamSelection{}, ErrMembershipIdentity
		}
		positions[v.Position] = v.EventID
		if position, ok := eventPositions[v.EventID]; ok && (position != v.Position || eventHashes[v.EventID] != v.EnvelopeDigest) {
			return StreamSelection{}, ErrMembershipIdentity
		}
		eventPositions[v.EventID] = v.Position
		eventHashes[v.EventID] = v.EnvelopeDigest
		if err := sortMembershipSet(v.Consumers, func(a, b EnrollmentConsumer) int { return strings.Compare(a.Name, b.Name) }); err != nil {
			return StreamSelection{}, err
		}
		for _, recipient := range v.Consumers {
			consumer, ok := consumers[recipient.Name]
			job := [2]string{v.EventID, recipient.Name}
			if !ok || jobs[job] || recipient.AdmissionID != consumer.AdmissionRootID || v.Position <= consumer.StartAfter || (consumer.EndInclusive != nil && v.Position > *consumer.EndInclusive) {
				return StreamSelection{}, ErrMembershipIdentity
			}
			jobs[job] = true
		}
		if v.Phase == "initial" {
			n := 0
			for _, consumer := range s.Consumers {
				if v.Position > consumer.StartAfter && (consumer.EndInclusive == nil || v.Position <= *consumer.EndInclusive) {
					n++
				}
			}
			if len(v.Consumers) != n {
				return StreamSelection{}, ErrMembershipIdentity
			}
		}
	}
	v, err := sealMembership(s, limits)
	return StreamSelection{v}, err
}

func NewTransitionRequest(in TransitionRequestSpec, limits MembershipLimits) (TransitionRequest, error) {
	s, err := membershipCopy(in, limits)
	if err != nil {
		return TransitionRequest{}, err
	}
	if s.Format != MembershipFormat || !s.Owner.Valid() || !membershipUUID(s.RequestID) || !validEvidence(s.Approval) || zeroDigest(s.EffectsDigest) || !slices.Contains([]string{"initial", "select", "admit-stream", "drain", "hold/resume"}, s.Action) {
		return TransitionRequest{}, ErrMembershipIdentity
	}
	if s.ExpectedHead == nil {
		if s.Action != "initial" || s.ResultEpoch != 1 {
			return TransitionRequest{}, ErrMembershipIdentity
		}
	} else {
		h := s.ExpectedHead
		if s.Action == "initial" || h.Epoch <= 0 || h.Epoch == math.MaxInt64 || s.ResultEpoch != h.Epoch+1 || !membershipUUID(h.RequestID) || h.RequestID == s.RequestID || !membershipLabel(h.CatalogVersion) || zeroDigest(h.CatalogDigest) || zeroDigest(h.SelectionDigest) {
			return TransitionRequest{}, ErrMembershipIdentity
		}
	}
	c, err := NewCatalog(s.Catalog, limits)
	if err != nil {
		return TransitionRequest{}, err
	}
	o, err := NewSelection(s.Selection, c, limits)
	if err != nil {
		return TransitionRequest{}, err
	}
	u, err := NewStreamUniverse(s.Universe, limits)
	if err != nil {
		return TransitionRequest{}, err
	}
	if s.Owner != c.Specification().Owner || s.Owner != u.Specification().Owner {
		return TransitionRequest{}, ErrMembershipIdentity
	}
	s.Catalog = c.Specification()
	s.Selection = o.Specification()
	s.Universe = u.Specification()
	if err := sortMembershipSet(s.Streams, func(a, b StreamSelectionSpec) int { return compareStream(a.Stream, b.Stream) }); err != nil {
		return TransitionRequest{}, err
	}
	if len(s.Streams) != len(s.Universe.Streams) {
		return TransitionRequest{}, ErrMembershipIdentity
	}
	eventsByID := map[string]MembershipStream{}
	enrollments := map[string]bool{}
	identityContext := streamIdentityContext(c, o, u)
	for i, stream := range s.Streams {
		if stream.Stream != s.Universe.Streams[i].Stream {
			return TransitionRequest{}, ErrMembershipIdentity
		}
		if s.ExpectedHead == nil && stream.ExpectedPrevious != nil {
			return TransitionRequest{}, ErrMembershipIdentity
		}
		if stream.ExpectedPrevious != nil && stream.ExpectedPrevious.RequestID != s.ExpectedHead.RequestID {
			return TransitionRequest{}, ErrMembershipIdentity
		}
		v, err := newStreamSelection(stream, identityContext, limits)
		if err != nil {
			return TransitionRequest{}, err
		}
		s.Streams[i] = v.Specification()
		for _, effect := range s.Streams[i].Backlog {
			if previous, ok := eventsByID[effect.EventID]; ok && previous != stream.Stream {
				return TransitionRequest{}, ErrMembershipIdentity
			}
			eventsByID[effect.EventID] = stream.Stream
			if enrollments[effect.EnrollmentID] {
				return TransitionRequest{}, ErrMembershipIdentity
			}
			enrollments[effect.EnrollmentID] = true
		}
	}
	v, err := sealMembership(s, limits)
	return TransitionRequest{v}, err
}

// CheckRequestIdentity is a pure same-ID/bytes comparison, not authoritative
// stored-history reconciliation or confirmation of an uncertain transaction.
func CheckRequestIdentity(retained, next TransitionRequest) error {
	if !retained.Valid() || !next.Valid() {
		return ErrMembershipIdentity
	}
	a, b := retained.Specification(), next.Specification()
	if a.Owner != b.Owner {
		return ErrMembershipIdentity
	}
	if a.RequestID == b.RequestID && !bytes.Equal(retained.canonical, next.canonical) {
		return ErrMembershipConflict
	}
	return nil
}

func DecodeCatalog(b []byte, l MembershipLimits) (Catalog, error) {
	var s CatalogSpec
	if err := decodeMembership(b, l, &s); err != nil {
		return Catalog{}, err
	}
	v, err := NewCatalog(s, l)
	if err == nil && !bytes.Equal(b, v.canonical) {
		err = ErrMembershipIdentity
	}
	if err != nil {
		return Catalog{}, err
	}
	return v, nil
}
func DecodeSelection(b []byte, c Catalog, l MembershipLimits) (Selection, error) {
	var s SelectionSpec
	if err := decodeMembership(b, l, &s); err != nil {
		return Selection{}, err
	}
	v, err := NewSelection(s, c, l)
	if err == nil && !bytes.Equal(b, v.canonical) {
		err = ErrMembershipIdentity
	}
	if err != nil {
		return Selection{}, err
	}
	return v, nil
}
func DecodeStreamUniverse(b []byte, l MembershipLimits) (StreamUniverse, error) {
	var s StreamUniverseSpec
	if err := decodeMembership(b, l, &s); err != nil {
		return StreamUniverse{}, err
	}
	v, err := NewStreamUniverse(s, l)
	if err == nil && !bytes.Equal(b, v.canonical) {
		err = ErrMembershipIdentity
	}
	if err != nil {
		return StreamUniverse{}, err
	}
	return v, nil
}
func DecodeStreamSelection(b []byte, c Catalog, s Selection, u StreamUniverse, l MembershipLimits) (StreamSelection, error) {
	var in StreamSelectionSpec
	if err := decodeMembership(b, l, &in); err != nil {
		return StreamSelection{}, err
	}
	v, err := NewStreamSelection(in, c, s, u, l)
	if err == nil && !bytes.Equal(b, v.canonical) {
		err = ErrMembershipIdentity
	}
	if err != nil {
		return StreamSelection{}, err
	}
	return v, nil
}
func DecodeTransitionRequest(b []byte, l MembershipLimits) (TransitionRequest, error) {
	var s TransitionRequestSpec
	if err := decodeMembership(b, l, &s); err != nil {
		return TransitionRequest{}, err
	}
	v, err := NewTransitionRequest(s, l)
	if err == nil && !bytes.Equal(b, v.canonical) {
		err = ErrMembershipIdentity
	}
	if err != nil {
		return TransitionRequest{}, err
	}
	return v, nil
}

func membershipText(s string) bool {
	return strings.TrimSpace(s) != "" && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
func membershipLabel(s string) bool {
	return s != "" && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
func zeroDigest(d MembershipDigest) bool { return d == MembershipDigest{} }
func validEvidence(e MembershipEvidence) bool {
	return membershipText(e.Reference) && !zeroDigest(e.Digest)
}
func validContract(c MembershipContract) bool {
	return membershipText(c.ID) && c.Version > 0 && !zeroDigest(c.Digest)
}
func validKind(k string) bool { return k == "projection" || k == "process" }
func membershipUUID(s string) bool {
	id, err := uuid.Parse(s)
	return err == nil && id != uuid.Nil && id.String() == s
}
func featureName(owner events.Owner, s string) bool {
	parts := strings.Split(s, "/")
	return len(parts) == 3 && parts[0] == string(owner) && membershipText(parts[1]) && membershipText(parts[2])
}

var membershipAggregate = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func validStream(s MembershipStream) bool {
	return s.SourceOwner.Valid() && membershipAggregate.MatchString(s.AggregateType) && membershipUUID(s.AggregateID)
}
func compareStream(a, b MembershipStream) int {
	if x := strings.Compare(string(a.SourceOwner), string(b.SourceOwner)); x != 0 {
		return x
	}
	if x := strings.Compare(a.AggregateType, b.AggregateType); x != 0 {
		return x
	}
	return strings.Compare(a.AggregateID, b.AggregateID)
}
func compareSchema(a, b MembershipSchema) int {
	if x := strings.Compare(a.EventType, b.EventType); x != 0 {
		return x
	}
	if a.Version < b.Version {
		return -1
	}
	if a.Version > b.Version {
		return 1
	}
	return 0
}
func compareMembershipContract(a, b MembershipContract) int {
	if x := strings.Compare(a.ID, b.ID); x != 0 {
		return x
	}
	if a.Version < b.Version {
		return -1
	}
	if a.Version > b.Version {
		return 1
	}
	return 0
}
func validMembershipSchema(owner events.Owner, s MembershipSchema) bool {
	scope := events.CompanyScopeTenantRequired
	if owner == events.OwnerIdentity {
		scope = events.CompanyScopeGlobalAllowed
	}
	_, err := events.NewEventSchema[struct{}](s.EventType, s.Version, "membership", scope, func(struct{}) error { return nil })
	return err == nil && strings.HasPrefix(s.EventType, string(owner)+".")
}
func validBootstrap(b BootstrapIdentity) bool {
	if !membershipUUID(b.ID) || !membershipUUID(b.RequestID) || !membershipUUID(b.AdmissionID) || b.StartAfter < 0 {
		return false
	}
	for _, s := range []string{b.AuthorityRef, b.ScopeRef, b.PurposeRef, b.SourceCheckpointRef} {
		if !membershipText(s) {
			return false
		}
	}
	if (b.Snapshot == nil) == (b.NoSnapshotContractRef == nil) {
		return false
	}
	if b.Snapshot != nil && !validEvidence(*b.Snapshot) {
		return false
	}
	if b.NoSnapshotContractRef != nil && !membershipText(*b.NoSnapshotContractRef) {
		return false
	}
	if b.StartAfter == 0 {
		return b.NewEmptyProofRef != nil && membershipText(*b.NewEmptyProofRef)
	}
	return b.NewEmptyProofRef == nil
}
func sortMembershipSet[T any](s []T, compare func(T, T) int) error {
	slices.SortFunc(s, compare)
	for i := 1; i < len(s); i++ {
		if compare(s[i-1], s[i]) == 0 {
			return ErrMembershipIdentity
		}
	}
	return nil
}

// The codec only handles the fixed DTOs above: no maps, arbitrary JSON, caller
// encoders or user code. Preflight measures before copying or allocating output.
func membershipJSON(v any) []byte {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		panic("invalid internal membership DTO")
	}
	return bytes.Clone(bytes.TrimSuffix(b.Bytes(), []byte{'\n'}))
}
func membershipCopy[T any](v T, l MembershipLimits) (T, error) {
	var zero T
	if err := measureMembership(reflect.ValueOf(v), l); err != nil {
		return zero, err
	}
	return cloneMembership(reflect.ValueOf(v)).Interface().(T), nil
}
func sealMembership[T any](v T, l MembershipLimits) (membershipValue[T], error) {
	if err := measureMembership(reflect.ValueOf(v), l); err != nil {
		return membershipValue[T]{}, err
	}
	b := membershipJSON(v)
	if len(b) > l.MaxBytes {
		return membershipValue[T]{}, ErrMembershipLimit
	}
	return membershipValue[T]{b, MembershipDigest(sha256.Sum256(b))}, nil
}
func cloneMembership(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Struct:
		r := reflect.New(v.Type()).Elem()
		for i := 0; i < v.NumField(); i++ {
			r.Field(i).Set(cloneMembership(v.Field(i)))
		}
		return r
	case reflect.Pointer:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		r := reflect.New(v.Type().Elem())
		r.Elem().Set(cloneMembership(v.Elem()))
		return r
	case reflect.Slice:
		r := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			r.Index(i).Set(cloneMembership(v.Index(i)))
		}
		return r
	default:
		return v
	}
}

func measureMembership(v reflect.Value, l MembershipLimits) error {
	if l.MaxBytes <= 0 || l.MaxValues <= 0 || l.MaxDepth <= 0 {
		return ErrMembershipLimit
	}
	used, values := 0, 0
	add := func(n int) error {
		if n < 0 || n > l.MaxBytes-used {
			return ErrMembershipLimit
		}
		used += n
		return nil
	}
	var walk func(reflect.Value, int) error
	walk = func(v reflect.Value, depth int) error {
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				values++
				if values > l.MaxValues {
					return ErrMembershipLimit
				}
				return add(4)
			}
			return walk(v.Elem(), depth)
		}
		if values == l.MaxValues {
			return ErrMembershipLimit
		}
		values++
		if v.Type() == reflect.TypeFor[MembershipDigest]() {
			return add(66)
		}
		switch v.Kind() {
		case reflect.String:
			s := v.String()
			if l.MaxBytes-used < 2 || len(s) > l.MaxBytes-used-2 {
				return ErrMembershipLimit
			}
			if !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
				return ErrMembershipIdentity
			}
			if err := add(2); err != nil {
				return err
			}
			for _, r := range s {
				n := utf8.RuneLen(r)
				switch r {
				case '"', '\\', '\b', '\f', '\n', '\r', '\t':
					n = 2
				case '\u2028', '\u2029':
					n = 6
				default:
					if r < 32 {
						n = 6
					}
				}
				if err := add(n); err != nil {
					return err
				}
			}
			return nil
		case reflect.Int, reflect.Int64:
			return add(len(strconv.FormatInt(v.Int(), 10)))
		case reflect.Uint32:
			return add(len(strconv.FormatUint(v.Uint(), 10)))
		case reflect.Struct:
			if depth == l.MaxDepth {
				return ErrMembershipLimit
			}
			if err := add(2); err != nil {
				return err
			}
			for i := 0; i < v.NumField(); i++ {
				name := v.Type().Field(i).Tag.Get("json")
				if name == "" {
					name = v.Type().Field(i).Name
				}
				if i > 0 {
					if err := add(1); err != nil {
						return err
					}
				}
				if err := walk(reflect.ValueOf(name), depth+1); err != nil {
					return err
				}
				if err := add(1); err != nil {
					return err
				}
				if err := walk(v.Field(i), depth+1); err != nil {
					return err
				}
			}
			return nil
		case reflect.Slice:
			if depth == l.MaxDepth {
				return ErrMembershipLimit
			}
			if v.Len() > l.MaxValues-values {
				return ErrMembershipLimit
			}
			if err := add(2); err != nil {
				return err
			}
			for i := 0; i < v.Len(); i++ {
				if i > 0 {
					if err := add(1); err != nil {
						return err
					}
				}
				if err := walk(v.Index(i), depth+1); err != nil {
					return err
				}
			}
			return nil
		default:
			return ErrMembershipIdentity
		}
	}
	return walk(v, 0)
}

func decodeMembership(b []byte, l MembershipLimits, out any) error {
	if l.MaxBytes <= 0 || l.MaxValues <= 0 || l.MaxDepth <= 0 || len(b) > l.MaxBytes {
		return ErrMembershipLimit
	}
	if !utf8.Valid(b) {
		return ErrMembershipIdentity
	}
	// Token preflight bounds container depth/value count and rejects duplicate
	// decoded keys before allocating the typed tree. Canonical re-encoding also
	// rejects lone-surrogate replacement, aliases, omitted fields and null sets.
	type frame struct {
		object, key bool
		seen        map[string]bool
	}
	stack := []frame{}
	count := 0
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	for {
		token, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ErrMembershipIdentity
		}
		delimiter, isDelimiter := token.(json.Delim)
		if isDelimiter && (delimiter == '}' || delimiter == ']') {
			if len(stack) == 0 {
				return ErrMembershipIdentity
			}
			stack = stack[:len(stack)-1]
			continue
		}
		if count == l.MaxValues {
			return ErrMembershipLimit
		}
		count++
		if len(stack) > 0 && stack[len(stack)-1].object {
			f := &stack[len(stack)-1]
			if f.key {
				s, ok := token.(string)
				if !ok || f.seen[s] {
					return ErrMembershipIdentity
				}
				f.seen[s] = true
				f.key = false
			} else {
				f.key = true
			}
		}
		if s, ok := token.(string); ok && strings.ContainsRune(s, 0) {
			return ErrMembershipIdentity
		}
		if isDelimiter {
			if len(stack) == l.MaxDepth {
				return ErrMembershipLimit
			}
			f := frame{object: delimiter == '{', key: delimiter == '{'}
			if f.object {
				f.seen = map[string]bool{}
			}
			stack = append(stack, f)
		}
	}
	if len(stack) != 0 {
		return ErrMembershipIdentity
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return fmt.Errorf("%w: typed manifest", ErrMembershipIdentity)
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrMembershipIdentity
	}
	return nil
}
