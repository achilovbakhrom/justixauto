package membershipqa

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
	"justixauto/pkg/projection"
)

// Only the outer adapter sees database/fences. Inbox's action port need not
// import projection or receive a pool, commit function, or arbitrary SQL.
type BootstrapAction interface { Install(context.Context) error }
type bootstrapAction func(context.Context) error
func (f bootstrapAction) Install(ctx context.Context) error { return f(ctx) }
type SnapshotPort interface { Restore(context.Context) error }

func bindBootstrap(tx *gorm.DB, fences eventstore.PreparedFences, c projection.Contract, proof projection.VerifiedBootstrap,
	bind func(*gorm.DB) (SnapshotPort, error), recheck func(context.Context, SnapshotPort, projection.VerifiedBootstrap) error) BootstrapAction {
	return bootstrapAction(func(ctx context.Context) error {
		cp, err := projection.NewCheckpoints(ctx, tx, fences, c, bind)
		if err != nil { return err }
		_, err = cp.InstallBootstrap(ctx, proof, recheck, func(ctx context.Context, p SnapshotPort) error { return p.Restore(ctx) })
		return err
	})
}

func contract(t *testing.T, generation string) projection.Contract {
	t.Helper()
	c, err := projection.NewContract(projection.ContractInput{
		LocalOwner: events.Owner("identity"), SourceOwner: events.Owner("inventory"),
		Consumer: "membership-qa", AggregateType: "vehicle", AggregateID: "8f9b12dd-f445-4a38-bc5f-c7b441be87bc",
		Kind: "projection", Generation: generation, ContractID: "fixture-only", ContractVersion: 1, ContractDigest: [32]byte{1},
	})
	if err != nil { t.Fatal(err) }
	return c
}

func TestOuterCompositionRejectsInvalidHandleBeforeFactory(t *testing.T) {
	for _, label := range []string{"ordinary", " ", "\t", " padded "} {
		t.Run(label, func(t *testing.T) {
			c := contract(t, label)
			for _, tx := range []*gorm.DB{nil, {}} {
				bound, rechecked := 0, 0
				action := bindBootstrap(tx, eventstore.PreparedFences{}, c, projection.VerifiedBootstrap{},
					func(*gorm.DB) (SnapshotPort, error) { bound++; return nil, nil },
					func(context.Context, SnapshotPort, projection.VerifiedBootstrap) error { rechecked++; return nil })
				if bound != 0 || rechecked != 0 { t.Fatal("eager composition") }
				if err := action.Install(context.Background()); err == nil || bound != 0 || rechecked != 0 {
					t.Fatalf("invalid handle reached callback: %v %d %d", err, bound, rechecked)
				}
			}
		})
	}
}

func TestZeroHighWaterRequiresSourceProofAndVerifier(t *testing.T) {
	c := contract(t, "membership-generation")
	b := projection.BootstrapInput{
		BootstrapID: "458b8c84-8c36-48dd-9542-9d9d38734b13", RequestID: "1e833cac-83c0-4d60-9c27-d26ba0cbd910",
		AuthorityRef: "fixture", ScopeRef: "fixture", PurposeRef: "fixture", SourceCheckpointRef: "fixture", NoSnapshotContractRef: "fixture",
	}
	calls := 0
	verify := func(context.Context, projection.BootstrapInput) error { calls++; return nil }
	if _, err := projection.VerifyBootstrap(context.Background(), c, b, verify); err == nil || calls != 0 { t.Fatal("zero inferred without proof") }
	b.NewEmptyProofRef = "explicit-fixture-only"
	if _, err := projection.VerifyBootstrap(context.Background(), c, b, nil); err == nil { t.Fatal("nil authority verifier") }
	if _, err := projection.VerifyBootstrap(context.Background(), c, b, verify); err != nil || calls != 1 { t.Fatal("explicit proof rejected", err) }
	denied := errors.New("denied fixture authority")
	if _, err := projection.VerifyBootstrap(context.Background(), c, b, func(context.Context, projection.BootstrapInput) error { return denied }); !errors.Is(err, denied) { t.Fatal("authority failure swallowed") }
}
