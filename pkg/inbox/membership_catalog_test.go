package inbox_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"testing"

	"justixauto/pkg/events"
	"justixauto/pkg/inbox"
)

var membershipLimits = inbox.MembershipLimits{MaxBytes: 1 << 20, MaxValues: 100000, MaxDepth: 64}

func mh(s string) inbox.MembershipDigest { return inbox.MembershipDigest(sha256.Sum256([]byte(s))) }
func me(s string) inbox.MembershipEvidence {
	return inbox.MembershipEvidence{Reference: s, Digest: mh(s)}
}
func mc(s string) inbox.MembershipContract {
	return inbox.MembershipContract{ID: s, Version: 1, Digest: mh(s)}
}
func mid(n int) string  { return fmt.Sprintf("00000000-0000-4000-8000-%012x", n) }
func ptr[T any](v T) *T { return &v }
func catalogFixture() inbox.CatalogSpec {
	s := inbox.CatalogSpec{Format: inbox.MembershipFormat, Owner: events.OwnerIdentity, Version: "release-2", Approval: me("catalog-approved"), BindingManifest: me("bindings-approved")}
	for i, kind := range []string{"projection", "process"} {
		id := "identity/" + kind + "/fixture"
		f := inbox.MembershipFeature{ID: id, Contract: mc(id)}
		if i == 1 {
			f.Requires = []string{"identity/projection/fixture"}
		}
		s.Features = append(s.Features, f)
		s.Claims = append(s.Claims, inbox.CatalogClaim{Format: inbox.MembershipFormat, Meaning: inbox.ConsumerMeaning{Format: inbox.MembershipFormat, Owner: events.OwnerIdentity, FeatureID: id, Name: "consumer-" + kind, Kind: kind, Generation: " \t", Subscription: mc("subscription-" + kind), SourceOwner: events.OwnerInventory, StreamID: "stream-selector-" + kind, Selector: mc("selector-" + kind), SchemaSemantics: me("semantics-" + kind), RoutingKeys: []string{"inventory.identity.fixture.v2", "inventory.identity.fixture.v1"}, Schemas: []inbox.MembershipSchema{{EventType: "inventory.fixture.changed.v2", Version: 2}, {EventType: "inventory.fixture.changed.v1", Version: 1}}}, Binding: inbox.ConsumerBinding{Exchange: "owner.inventory", Queue: "identity.ingress", Approval: me("binding-" + kind)}})
	}
	return s
}
func mustCatalog(t testing.TB, s inbox.CatalogSpec) inbox.Catalog {
	t.Helper()
	v, err := inbox.NewCatalog(s, membershipLimits)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func frozenFixture(c inbox.Catalog) ([]inbox.FrozenFeatureClaims, []inbox.MembershipSupplement) {
	s := c.Specification()
	var f []inbox.FrozenFeatureClaims
	var extras []inbox.MembershipSupplement
	for _, feature := range s.Features {
		v := inbox.FrozenFeatureClaims{Owner: s.Owner, ID: feature.ID, Requires: feature.Requires}
		for _, claim := range s.Claims {
			m := claim.Meaning
			if m.FeatureID != feature.ID {
				continue
			}
			v.Consumers = append(v.Consumers, inbox.FrozenConsumerClaim{Name: m.Name, Kind: m.Kind, Subscription: inbox.FrozenSubscriptionClaim{ContractID: m.Subscription.ID, SourceOwner: m.SourceOwner, StreamID: m.StreamID, Exchange: claim.Binding.Exchange, Queue: claim.Binding.Queue, RoutingKeys: m.RoutingKeys, Schemas: m.Schemas}})
			extras = append(extras, inbox.MembershipSupplement{Meaning: m, Binding: claim.Binding})
		}
		f = append(f, v)
	}
	return f, extras
}
func requestFixture(t testing.TB) inbox.TransitionRequestSpec {
	t.Helper()
	c := mustCatalog(t, catalogFixture())
	cs := c.Specification()
	os := inbox.SelectionSpec{Format: inbox.MembershipFormat, Owner: cs.Owner, CatalogVersion: cs.Version, CatalogDigest: c.Digest(), Approval: me("selection-approved")}
	for _, claim := range cs.Claims {
		_, hash, err := c.ClaimBytes(claim.Meaning.Name)
		if err != nil {
			t.Fatal(err)
		}
		os.Consumers = append(os.Consumers, inbox.SelectedConsumer{Name: claim.Meaning.Name, ClaimDigest: hash, State: "active", Disposition: me("active-" + claim.Meaning.Name)})
	}
	o, err := inbox.NewSelection(os, c, membershipLimits)
	if err != nil {
		t.Fatal(err)
	}
	us := inbox.StreamUniverseSpec{Format: inbox.MembershipFormat, Owner: cs.Owner, Authority: me("finite-source-universe")}
	for i := 1; i <= 2; i++ {
		v := inbox.UniverseMember{Stream: inbox.MembershipStream{SourceOwner: events.OwnerInventory, AggregateType: "vehicle", AggregateID: mid(i)}, SourceVerification: me("source-verified"), Disposition: me("retained-stream")}
		for _, claim := range cs.Claims {
			v.Selectors = append(v.Selectors, claim.Meaning.Selector)
		}
		us.Streams = append(us.Streams, v)
	}
	u, err := inbox.NewStreamUniverse(us, membershipLimits)
	if err != nil {
		t.Fatal(err)
	}
	r := inbox.TransitionRequestSpec{Format: inbox.MembershipFormat, Owner: cs.Owner, RequestID: mid(50), Action: "initial", ResultEpoch: 1, Approval: me("transition-approved"), Catalog: cs, Selection: o.Specification(), Universe: u.Specification(), EffectsDigest: mh("complete-effects")}
	for i, member := range r.Universe.Streams {
		s := inbox.StreamSelectionSpec{Format: inbox.MembershipFormat, Owner: cs.Owner, Stream: member.Stream, CatalogVersion: cs.Version, CatalogDigest: c.Digest(), SelectionDigest: o.Digest(), UniverseDigest: u.Digest(), Authority: me("stream-scope-purpose-authority"), SourceProof: member.SourceVerification, Boundary: int64(1 - i)}
		for j, claim := range cs.Claims {
			m := claim.Meaning
			_, hash, _ := c.ClaimBytes(m.Name)
			if i == 1 {
				s.Exclusions = append(s.Exclusions, inbox.ConsumerExclusion{Name: m.Name, ClaimDigest: hash, Authority: me("explicit-exclusion")})
				continue
			}
			admission := mid(100 + j)
			consumer := inbox.StreamConsumer{Name: m.Name, ClaimDigest: hash, Kind: m.Kind, Generation: m.Generation, AdmissionRootID: admission, AdmissionTipID: admission, Bootstrap: inbox.BootstrapIdentity{ID: mid(200 + j), RequestID: r.RequestID, AdmissionID: admission, AuthorityRef: "source-authority", ScopeRef: "source-scope", PurposeRef: "source-purpose", SourceCheckpointRef: "issued-h0", NoSnapshotContractRef: ptr("approved-no-snapshot"), NewEmptyProofRef: ptr("source-proved-empty")}, Disposition: me("retain"), EffectDigest: mh("bootstrap-effect")}
			if m.Kind == "process" {
				consumer.ProcessCatchUp = ptr(me("process-specific-catchup"))
			}
			s.Consumers = append(s.Consumers, consumer)
		}
		if i == 1 {
			s.ZeroApplicable = ptr(me("explicit-zero-applicable"))
		} else {
			effect := inbox.EnrollmentEffect{EventID: mid(300), EnrollmentID: mid(400), Position: 1, EnvelopeDigest: mh("original-envelope"), Phase: "initial", EffectDigest: mh("complete-enrollment-jobs")}
			for _, consumer := range s.Consumers {
				effect.Consumers = append(effect.Consumers, inbox.EnrollmentConsumer{Name: consumer.Name, AdmissionID: consumer.AdmissionRootID})
			}
			s.Backlog = append(s.Backlog, effect)
		}
		r.Streams = append(r.Streams, s)
	}
	return r
}
func mustRequest(t testing.TB, s inbox.TransitionRequestSpec) inbox.TransitionRequest {
	t.Helper()
	v, err := inbox.NewTransitionRequest(s, membershipLimits)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMembershipCanonicalGoldenEmptyCatalog(t *testing.T) {
	d := inbox.MembershipDigest{}
	for i := range d {
		d[i] = 0xab
	}
	s := inbox.CatalogSpec{Format: inbox.MembershipFormat, Owner: events.OwnerIdentity, Version: " \t<&>", Approval: inbox.MembershipEvidence{Reference: "approved", Digest: d}, BindingManifest: inbox.MembershipEvidence{Reference: "bindings", Digest: d}}
	c := mustCatalog(t, s)
	h := strings.Repeat("ab", 32)
	want := `{"format":"membership-v1","owner":"identity","version":" \t<&>","approval":{"reference":"approved","digest":"` + h + `"},"binding_manifest":{"reference":"bindings","digest":"` + h + `"},"features":[],"claims":[]}`
	if string(c.Bytes()) != want {
		t.Fatalf("wire mismatch\n%s\n%s", c.Bytes(), want)
	}
	if c.Digest() != inbox.MembershipDigest(sha256.Sum256([]byte(want))) {
		t.Fatal("digest is not exact original canonical bytes")
	}
	decoded, err := inbox.DecodeCatalog([]byte(want), membershipLimits)
	if err != nil || decoded.Digest() != c.Digest() {
		t.Fatalf("golden decode: %v", err)
	}
	l := membershipLimits
	l.MaxBytes = len(want)
	if _, err := inbox.NewCatalog(s, l); err != nil {
		t.Fatalf("exact byte budget: %v", err)
	}
	l.MaxBytes--
	if _, err := inbox.NewCatalog(s, l); !errors.Is(err, inbox.ErrMembershipLimit) {
		t.Fatalf("over budget: %v", err)
	}
}

func TestMembershipCanonicalPermutationAndRoundTrip(t *testing.T) {
	base := mustRequest(t, requestFixture(t))
	want := base.Bytes()
	rng := rand.New(rand.NewSource(947))
	for range 100 {
		s := base.Specification()
		rng.Shuffle(len(s.Catalog.Features), func(i, j int) {
			s.Catalog.Features[i], s.Catalog.Features[j] = s.Catalog.Features[j], s.Catalog.Features[i]
		})
		rng.Shuffle(len(s.Catalog.Claims), func(i, j int) { s.Catalog.Claims[i], s.Catalog.Claims[j] = s.Catalog.Claims[j], s.Catalog.Claims[i] })
		for i := range s.Catalog.Claims {
			rng.Shuffle(len(s.Catalog.Claims[i].Meaning.RoutingKeys), func(a, b int) { v := s.Catalog.Claims[i].Meaning.RoutingKeys; v[a], v[b] = v[b], v[a] })
			slices.Reverse(s.Catalog.Claims[i].Meaning.Schemas)
		}
		slices.Reverse(s.Selection.Consumers)
		slices.Reverse(s.Universe.Streams)
		for i := range s.Universe.Streams {
			slices.Reverse(s.Universe.Streams[i].Selectors)
		}
		slices.Reverse(s.Streams)
		for i := range s.Streams {
			slices.Reverse(s.Streams[i].Consumers)
			slices.Reverse(s.Streams[i].Exclusions)
			for j := range s.Streams[i].Backlog {
				slices.Reverse(s.Streams[i].Backlog[j].Consumers)
			}
		}
		got := mustRequest(t, s)
		if !bytes.Equal(got.Bytes(), want) || got.Digest() != base.Digest() {
			t.Fatal("set permutation changed identity")
		}
	}
	decoded, err := inbox.DecodeTransitionRequest(want, membershipLimits)
	if err != nil || decoded.Digest() != base.Digest() {
		t.Fatal(err)
	}
	s := base.Specification()
	c := mustCatalog(t, s.Catalog)
	o, err := inbox.NewSelection(s.Selection, c, membershipLimits)
	if err != nil {
		t.Fatal(err)
	}
	u, err := inbox.NewStreamUniverse(s.Universe, membershipLimits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inbox.DecodeSelection(o.Bytes(), c, membershipLimits); err != nil {
		t.Fatal(err)
	}
	if _, err := inbox.DecodeStreamUniverse(u.Bytes(), membershipLimits); err != nil {
		t.Fatal(err)
	}
	for _, v := range s.Streams {
		stream, err := inbox.NewStreamSelection(v, c, o, u, membershipLimits)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := inbox.DecodeStreamSelection(stream.Bytes(), c, o, u, membershipLimits); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMembershipRawManifestRejectsNormalization(t *testing.T) {
	r := mustRequest(t, requestFixture(t))
	raw := string(r.Bytes())
	cases := map[string]string{
		"leading-space": " " + raw, "trailing-space": raw + "\n", "second-value": raw + "{}",
		"duplicate-key":     strings.Replace(raw, `"owner":"identity"`, `"owner":"identity","owner":"identity"`, 1),
		"decoded-duplicate": strings.Replace(raw, `"owner":"identity"`, `"owner":"identity","\u006fwner":"identity"`, 1),
		"unknown-field":     strings.Replace(raw, `"format":`, `"unknown":null,"format":`, 1),
		"case-alias":        strings.Replace(raw, `"owner":`, `"Owner":`, 1),
		"omitted-field":     strings.Replace(raw, `"expected_head":null,`, "", 1),
		"escaped-key":       strings.Replace(raw, `"owner":`, `"\u006fwner":`, 1),
		"lone-high":         strings.Replace(raw, `"release-2"`, `"\ud800"`, 1),
		"lone-low":          strings.Replace(raw, `"release-2"`, `"\udc00"`, 1),
		"invalid-utf8":      strings.Replace(raw, `release-2`, string([]byte{0xff}), 1),
		"nul":               strings.Replace(raw, `"release-2"`, `"bad\u0000label"`, 1),
		"exponent":          strings.Replace(raw, `"result_epoch":1`, `"result_epoch":1e0`, 1),
		"fraction":          strings.Replace(raw, `"result_epoch":1`, `"result_epoch":1.0`, 1),
		"negative-zero":     strings.Replace(raw, `"boundary":0`, `"boundary":-0`, 1),
		"integer-overflow":  strings.Replace(raw, `"result_epoch":1`, `"result_epoch":9223372036854775808`, 1),
	}
	d := r.Specification().Approval.Digest
	hexDigest := hex.EncodeToString(d[:])
	cases["hex-uppercase"] = strings.Replace(raw, hexDigest, strings.ToUpper(hexDigest), 1)
	cases["null-array"] = strings.Replace(raw, `"backlog":[]`, `"backlog":null`, 1)
	cases["null-required-string"] = strings.Replace(raw, `"format":"membership-v1"`, `"format":null`, 1)
	cases["reordered-fields"] = strings.Replace(raw, `"format":"membership-v1","owner":"identity"`, `"owner":"identity","format":"membership-v1"`, 1)
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if input == raw {
				t.Fatal("mutation did not apply")
			}
			value, err := inbox.DecodeTransitionRequest([]byte(input), membershipLimits)
			if err == nil || value.Valid() || value.Digest() != (inbox.MembershipDigest{}) {
				t.Fatalf("noncanonical manifest accepted: %v", err)
			}
		})
	}
}

func TestMembershipRejectsIncompleteAndContradictorySets(t *testing.T) {
	base := mustRequest(t, requestFixture(t))
	cases := map[string]func(*inbox.TransitionRequestSpec){
		"zero-request":            func(s *inbox.TransitionRequestSpec) { s.RequestID = "00000000-0000-0000-0000-000000000000" },
		"noncanonical-uuid":       func(s *inbox.TransitionRequestSpec) { s.RequestID = strings.Replace(s.RequestID, "0032", "00AB", 1) },
		"wrong-owner":             func(s *inbox.TransitionRequestSpec) { s.Owner = events.OwnerInventory },
		"initial-needs-epoch-one": func(s *inbox.TransitionRequestSpec) { s.ResultEpoch = 2 },
		"noninitial-needs-head":   func(s *inbox.TransitionRequestSpec) { s.Action = "select" },
		"unknown-action":          func(s *inbox.TransitionRequestSpec) { s.Action = "repair" },
		"missing-universe-member": func(s *inbox.TransitionRequestSpec) { s.Streams = s.Streams[:1] },
		"duplicate-stream":        func(s *inbox.TransitionRequestSpec) { s.Streams[1] = s.Streams[0] },
		"duplicate-feature": func(s *inbox.TransitionRequestSpec) {
			s.Catalog.Features = append(s.Catalog.Features, s.Catalog.Features[0])
		},
		"missing-feature": func(s *inbox.TransitionRequestSpec) { s.Catalog.Features = s.Catalog.Features[:1] },
		"dependency-cycle": func(s *inbox.TransitionRequestSpec) {
			s.Catalog.Features[0].Requires = []string{s.Catalog.Features[0].ID}
		},
		"duplicate-consumer": func(s *inbox.TransitionRequestSpec) { s.Catalog.Claims = append(s.Catalog.Claims, s.Catalog.Claims[0]) },
		"duplicate-routing": func(s *inbox.TransitionRequestSpec) {
			m := &s.Catalog.Claims[0].Meaning
			m.RoutingKeys = append(m.RoutingKeys, m.RoutingKeys[0])
		},
		"duplicate-schema": func(s *inbox.TransitionRequestSpec) {
			m := &s.Catalog.Claims[0].Meaning
			m.Schemas = append(m.Schemas, m.Schemas[0])
		},
		"wrong-schema-owner": func(s *inbox.TransitionRequestSpec) {
			s.Catalog.Claims[0].Meaning.Schemas[0].EventType = "retail.fixture.changed.v1"
		},
		"schema-version":     func(s *inbox.TransitionRequestSpec) { s.Catalog.Claims[0].Meaning.Schemas[0].Version = 0 },
		"missing-generation": func(s *inbox.TransitionRequestSpec) { s.Catalog.Claims[0].Meaning.Generation = "" },
		"invalid-input-utf8": func(s *inbox.TransitionRequestSpec) { s.Catalog.Version = string([]byte{255}) },
		"nul-input":          func(s *inbox.TransitionRequestSpec) { s.Catalog.Version = "a\x00b" },
		"zero-contract-digest": func(s *inbox.TransitionRequestSpec) {
			s.Catalog.Claims[0].Meaning.Subscription.Digest = inbox.MembershipDigest{}
		},
		"selection-claim-hash": func(s *inbox.TransitionRequestSpec) { s.Selection.Consumers[0].ClaimDigest = mh("wrong") },
		"implicit-empty-selection": func(s *inbox.TransitionRequestSpec) {
			s.Selection.Consumers = nil
			s.Selection.Approval = inbox.MembershipEvidence{}
		},
		"unknown-disposition":         func(s *inbox.TransitionRequestSpec) { s.Selection.Consumers[0].State = "complete" },
		"universe-digest":             func(s *inbox.TransitionRequestSpec) { s.Streams[0].UniverseDigest = mh("wrong") },
		"omitted-applicable-consumer": func(s *inbox.TransitionRequestSpec) { s.Streams[0].Consumers = s.Streams[0].Consumers[:1] },
		"omitted-exclusion":           func(s *inbox.TransitionRequestSpec) { s.Streams[1].Exclusions = s.Streams[1].Exclusions[:1] },
		"zero-without-authority":      func(s *inbox.TransitionRequestSpec) { s.Streams[1].ZeroApplicable = nil },
		"included-and-excluded":       func(s *inbox.TransitionRequestSpec) { s.Streams[0].Exclusions = s.Streams[1].Exclusions },
		"missing-source-empty-proof":  func(s *inbox.TransitionRequestSpec) { s.Streams[0].Consumers[0].Bootstrap.NewEmptyProofRef = nil },
		"snapshot-and-no-snapshot": func(s *inbox.TransitionRequestSpec) {
			s.Streams[0].Consumers[0].Bootstrap.Snapshot = ptr(me("snapshot"))
		},
		"bootstrap-admission-mismatch": func(s *inbox.TransitionRequestSpec) { s.Streams[0].Consumers[0].Bootstrap.AdmissionID = mid(999) },
		"range-negative":               func(s *inbox.TransitionRequestSpec) { s.Streams[0].Consumers[0].StartAfter = -1 },
		"range-reversed":               func(s *inbox.TransitionRequestSpec) { s.Streams[0].Consumers[0].EndInclusive = ptr(int64(-1)) },
		"missing-process-proof": func(s *inbox.TransitionRequestSpec) {
			for i := range s.Streams[0].Consumers {
				if s.Streams[0].Consumers[i].Kind == "process" {
					s.Streams[0].Consumers[i].ProcessCatchUp = nil
				}
			}
		},
		"wrong-retained-generation": func(s *inbox.TransitionRequestSpec) { s.Streams[0].Consumers[0].Generation = "other" },
		"backlog-beyond-boundary":   func(s *inbox.TransitionRequestSpec) { s.Streams[0].Backlog[0].Position = 2 },
		"duplicate-enrollment": func(s *inbox.TransitionRequestSpec) {
			s.Streams[0].Backlog = append(s.Streams[0].Backlog, s.Streams[0].Backlog[0])
		},
		"missing-initial-job": func(s *inbox.TransitionRequestSpec) {
			s.Streams[0].Backlog[0].Consumers = s.Streams[0].Backlog[0].Consumers[:1]
		},
		"wrong-admission-job": func(s *inbox.TransitionRequestSpec) { s.Streams[0].Backlog[0].Consumers[0].AdmissionID = mid(998) },
		"duplicate-job": func(s *inbox.TransitionRequestSpec) {
			v := &s.Streams[0].Backlog[0]
			v.Consumers = append(v.Consumers, v.Consumers[0])
		},
		"empty-late-enrollment": func(s *inbox.TransitionRequestSpec) {
			v := &s.Streams[0].Backlog[0]
			v.Phase = "late"
			v.Consumers = nil
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := base.Specification()
			mutate(&s)
			v, err := inbox.NewTransitionRequest(s, membershipLimits)
			if err == nil || v.Valid() {
				t.Fatal("invalid complete snapshot accepted")
			}
		})
	}
}

func TestMembershipFrozenClaimSupplementExactness(t *testing.T) {
	c := mustCatalog(t, catalogFixture())
	f, s := frozenFixture(c)
	if err := c.MatchClaims(f, s, membershipLimits); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func([]inbox.FrozenFeatureClaims, []inbox.MembershipSupplement) ([]inbox.FrozenFeatureClaims, []inbox.MembershipSupplement){
		"missing-feature": func(f []inbox.FrozenFeatureClaims, s []inbox.MembershipSupplement) ([]inbox.FrozenFeatureClaims, []inbox.MembershipSupplement) {
			return f[:1], s
		},
		"missing-supplement": func(f []inbox.FrozenFeatureClaims, s []inbox.MembershipSupplement) ([]inbox.FrozenFeatureClaims, []inbox.MembershipSupplement) {
			return f, s[:1]
		},
		"extra-supplement": func(f []inbox.FrozenFeatureClaims, s []inbox.MembershipSupplement) ([]inbox.FrozenFeatureClaims, []inbox.MembershipSupplement) {
			return f, append(s, s[0])
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f, s := frozenFixture(c)
			f, s = mutate(f, s)
			if err := c.MatchClaims(f, s, membershipLimits); err == nil {
				t.Fatal("incomplete claims accepted")
			}
		})
	}
	for name, mutate := range map[string]func(*inbox.FrozenFeatureClaims){
		"owner": func(f *inbox.FrozenFeatureClaims) { f.Owner = events.OwnerRetail }, "feature": func(f *inbox.FrozenFeatureClaims) { f.ID = "identity/process/other" }, "dependencies": func(f *inbox.FrozenFeatureClaims) { f.Requires = nil },
		"consumer": func(f *inbox.FrozenFeatureClaims) { f.Consumers[0].Name = "other" }, "kind": func(f *inbox.FrozenFeatureClaims) { f.Consumers[0].Kind = "projection" }, "contract": func(f *inbox.FrozenFeatureClaims) { f.Consumers[0].Subscription.ContractID = "other" }, "source": func(f *inbox.FrozenFeatureClaims) { f.Consumers[0].Subscription.SourceOwner = events.OwnerRetail }, "stream": func(f *inbox.FrozenFeatureClaims) { f.Consumers[0].Subscription.StreamID = "other" }, "exchange": func(f *inbox.FrozenFeatureClaims) { f.Consumers[0].Subscription.Exchange = "other" }, "queue": func(f *inbox.FrozenFeatureClaims) { f.Consumers[0].Subscription.Queue = "other" }, "route": func(f *inbox.FrozenFeatureClaims) { f.Consumers[0].Subscription.RoutingKeys[0] = "other" }, "schema": func(f *inbox.FrozenFeatureClaims) { f.Consumers[0].Subscription.Schemas[0].Version = 9 },
	} {
		t.Run(name, func(t *testing.T) {
			f, s := frozenFixture(c)
			mutate(&f[0])
			if err := c.MatchClaims(f, s, membershipLimits); err == nil {
				t.Fatal("overlapping frozen field changed")
			}
		})
	}
	for name, mutate := range map[string]func(*inbox.MembershipSupplement){"generation": func(s *inbox.MembershipSupplement) { s.Meaning.Generation = "other" }, "numeric-contract-version": func(s *inbox.MembershipSupplement) { s.Meaning.Subscription.Version++ }, "contract-digest": func(s *inbox.MembershipSupplement) { s.Meaning.Subscription.Digest = mh("other") }, "selector": func(s *inbox.MembershipSupplement) { s.Meaning.Selector = mc("other") }, "schema-semantics": func(s *inbox.MembershipSupplement) { s.Meaning.SchemaSemantics = me("other") }, "binding-approval": func(s *inbox.MembershipSupplement) { s.Binding.Approval = me("other") }} {
		t.Run(name, func(t *testing.T) {
			f, s := frozenFixture(c)
			mutate(&s[0])
			if err := c.MatchClaims(f, s, membershipLimits); err == nil {
				t.Fatal("supplement identity mismatch accepted")
			}
		})
	}
}

func TestMembershipRetainedMeaningVersionAndRequestComparisons(t *testing.T) {
	c := mustCatalog(t, catalogFixture())
	s := c.Specification()
	s.Claims[0].Binding.Queue = "new-approved-queue"
	s.BindingManifest = me("new-binding")
	sameVersion := mustCatalog(t, s)
	if !errors.Is(inbox.CheckCatalogIdentity(c, sameVersion), inbox.ErrMembershipConflict) {
		t.Fatal("same version changed bytes accepted")
	}
	s.Version = "\t"
	next := mustCatalog(t, s)
	if err := inbox.CheckCatalogIdentity(c, next); err != nil {
		t.Fatalf("binding-only version change: %v", err)
	}
	s.Claims[0].Meaning.Generation = "changed-meaning"
	if !errors.Is(inbox.CheckCatalogIdentity(c, mustCatalog(t, s)), inbox.ErrMembershipConflict) {
		t.Fatal("name reused with changed meaning")
	}
	r := mustRequest(t, requestFixture(t))
	rs := r.Specification()
	rs.Approval = me("different-approval")
	changed := mustRequest(t, rs)
	if !errors.Is(inbox.CheckRequestIdentity(r, changed), inbox.ErrMembershipConflict) {
		t.Fatal("same request changed bytes accepted")
	}
	if err := inbox.CheckRequestIdentity(r, r); err != nil {
		t.Fatal(err)
	}
	// Version-only/no-new-consumer work still has an explicit next epoch and UUID.
	rs = r.Specification()
	rs.RequestID = mid(51)
	rs.Action = "select"
	rs.ExpectedHead = &inbox.MembershipHeadIdentity{Epoch: 1, RequestID: r.Specification().RequestID, CatalogVersion: rs.Catalog.Version, CatalogDigest: rs.Selection.CatalogDigest, SelectionDigest: rs.Streams[0].SelectionDigest}
	rs.ResultEpoch = 2
	for i := range rs.Streams {
		rs.Streams[i].Backlog = nil
		rs.Streams[i].ExpectedPrevious = &inbox.StreamSelectionIdentity{RequestID: r.Specification().RequestID, Digest: mh("retained-stream-selection")}
	}
	nextRequest := mustRequest(t, rs)
	if nextRequest.Digest() == r.Digest() || nextRequest.Specification().ResultEpoch != 2 {
		t.Fatal("zero-delta transition lost identity")
	}
	rs.ExpectedHead.Epoch = math.MaxInt64
	rs.ResultEpoch = math.MinInt64
	if _, err := inbox.NewTransitionRequest(rs, membershipLimits); err == nil {
		t.Fatal("epoch overflow accepted")
	}
	var zero inbox.Catalog
	if zero.Valid() || zero.Bytes() != nil || zero.Digest() != (inbox.MembershipDigest{}) || inbox.CheckCatalogIdentity(zero, c) == nil {
		t.Fatal("zero catalog accepted")
	}
	if inbox.CheckRequestIdentity(inbox.TransitionRequest{}, r) == nil {
		t.Fatal("zero request accepted")
	}
}

func TestMembershipEmptyUniverseAndIntegerExtremes(t *testing.T) {
	cs := catalogFixture()
	cs.Features = nil
	cs.Claims = nil
	cs.Version = " "
	c := mustCatalog(t, cs)
	o, err := inbox.NewSelection(inbox.SelectionSpec{Format: inbox.MembershipFormat, Owner: cs.Owner, CatalogVersion: cs.Version, CatalogDigest: c.Digest(), Approval: me("explicit-empty-selection")}, c, membershipLimits)
	if err != nil {
		t.Fatal(err)
	}
	u, err := inbox.NewStreamUniverse(inbox.StreamUniverseSpec{Format: inbox.MembershipFormat, Owner: cs.Owner, Authority: me("source-proved-empty-universe")}, membershipLimits)
	if err != nil {
		t.Fatal(err)
	}
	s := inbox.TransitionRequestSpec{Format: inbox.MembershipFormat, Owner: cs.Owner, RequestID: mid(70), Action: "initial", ResultEpoch: 1, Approval: me("explicit-initial-empty"), Catalog: c.Specification(), Selection: o.Specification(), Universe: u.Specification(), EffectsDigest: mh("zero-effects")}
	first := mustRequest(t, s)
	if bytes.Contains(first.Bytes(), []byte(`"streams":null`)) {
		t.Fatal("empty universe became absent")
	}
	s.RequestID = mid(71)
	s.Action = "select"
	s.ResultEpoch = math.MaxInt64
	s.ExpectedHead = &inbox.MembershipHeadIdentity{Epoch: math.MaxInt64 - 1, RequestID: mid(69), CatalogVersion: cs.Version, CatalogDigest: c.Digest(), SelectionDigest: o.Digest()}
	last := mustRequest(t, s)
	if !bytes.Contains(last.Bytes(), []byte(`"result_epoch":9223372036854775807`)) {
		t.Fatal("signed bigint boundary rounded")
	}
	if _, err := inbox.DecodeTransitionRequest(last.Bytes(), membershipLimits); err != nil {
		t.Fatal(err)
	}
	cs = catalogFixture()
	cs.Claims[0].Meaning.Subscription.Version = math.MaxUint32
	c = mustCatalog(t, cs)
	if !bytes.Contains(c.Bytes(), []byte(`"version":4294967295`)) {
		t.Fatal("contract revision rounded")
	}
	r := requestFixture(t)
	for i := range r.Streams[0].Consumers {
		v := &r.Streams[0].Consumers[i]
		v.StartAfter = math.MaxInt64
		v.Bootstrap.StartAfter = math.MaxInt64
		v.Bootstrap.NewEmptyProofRef = nil
		v.EndInclusive = ptr(int64(math.MaxInt64))
	}
	r.Streams[0].Boundary = math.MaxInt64
	r.Streams[0].Backlog = nil
	if _, err := inbox.DecodeTransitionRequest(mustRequest(t, r).Bytes(), membershipLimits); err != nil {
		t.Fatal(err)
	}
}

func TestMembershipDefensiveCopiesAndConcurrentReads(t *testing.T) {
	in := requestFixture(t)
	r := mustRequest(t, in)
	original := r.Bytes()
	in.Catalog.Claims[0].Meaning.RoutingKeys[0] = "mutated-input"
	*in.Streams[0].Consumers[0].Bootstrap.NoSnapshotContractRef = "mutated-input"
	in.Streams[0].Backlog[0].Consumers[0].Name = "mutated-input"
	if !bytes.Equal(r.Bytes(), original) {
		t.Fatal("input mutation leaked")
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			for range 20 {
				s := r.Specification()
				s.Catalog.Claims[0].Meaning.Schemas[0].EventType = "mutated-output"
				*s.Streams[0].Consumers[0].Bootstrap.NoSnapshotContractRef = "mutated-output"
				b := r.Bytes()
				b[0] = '!'
				if !bytes.Equal(r.Bytes(), original) {
					t.Error("returned value mutation leaked")
				}
				decoded, err := inbox.DecodeTransitionRequest(original, membershipLimits)
				if err != nil || decoded.Digest() != r.Digest() {
					t.Errorf("concurrent decode: %v", err)
				}
			}
		})
	}
	wg.Wait()
}

func TestMembershipExplicitResourceCeilings(t *testing.T) {
	r := mustRequest(t, requestFixture(t))
	for _, l := range []inbox.MembershipLimits{{}, {MaxBytes: len(r.Bytes()) - 1, MaxValues: 100000, MaxDepth: 64}, {MaxBytes: 1 << 20, MaxValues: 3, MaxDepth: 64}, {MaxBytes: 1 << 20, MaxValues: 100000, MaxDepth: 2}} {
		if _, err := inbox.DecodeTransitionRequest(r.Bytes(), l); !errors.Is(err, inbox.ErrMembershipLimit) {
			t.Fatalf("decode limit: %v", err)
		}
		if _, err := inbox.NewTransitionRequest(r.Specification(), l); !errors.Is(err, inbox.ErrMembershipLimit) {
			t.Fatalf("constructor limit: %v", err)
		}
	}
	deep := []byte(strings.Repeat("[", 65) + "0" + strings.Repeat("]", 65))
	if _, err := inbox.DecodeCatalog(deep, membershipLimits); !errors.Is(err, inbox.ErrMembershipLimit) {
		t.Fatalf("depth preflight: %v", err)
	}
	large := catalogFixture()
	large.Version = strings.Repeat("界", 400000)
	if _, err := inbox.NewCatalog(large, membershipLimits); !errors.Is(err, inbox.ErrMembershipLimit) {
		t.Fatalf("byte count overflow/limit: %v", err)
	}
}

func FuzzMembershipCatalogCanonical(f *testing.F) {
	c := mustCatalog(f, catalogFixture())
	f.Add(c.Bytes())
	f.Add([]byte(`{"format":"membership-v1","owner":"identity","owner":"identity"}`))
	f.Add([]byte(`{"version":"\ud800"}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		v, err := inbox.DecodeCatalog(b, membershipLimits)
		if err != nil {
			if v.Valid() {
				t.Fatal("error returned valid value")
			}
			return
		}
		if !bytes.Equal(v.Bytes(), b) || v.Digest() != inbox.MembershipDigest(sha256.Sum256(b)) {
			t.Fatal("accepted noncanonical or wrong digest")
		}
		var decoded any
		if err := json.Unmarshal(b, &decoded); err != nil {
			t.Fatal(err)
		}
	})
}
