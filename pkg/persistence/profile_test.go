package persistence_test

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"justixauto/pkg/persistence"
)

func hash(s string) persistence.Digest { return persistence.Digest(sha256.Sum256([]byte(s))) }
func spec() persistence.Specification {
	base := persistence.ArtifactIdentity{Version: 1, Filename: "migrations/0001_mechanics.up.sql", SHA256: hash("base")}
	f := persistence.FeatureIdentity{ID: "synthetic.feature", Revision: 1, SHA256: hash("contract")}
	return persistence.Specification{Owner: "identity", Database: "justix_identity", RuntimeRole: "justix_identity_runtime", FormatRevision: 1, Head: 12, HistoryRevision: 1, HistorySHA256: hash("history"),
		Baseline:  persistence.Baseline{Kind: "verified-installation", EvidenceRef: "fixture-execution", ApprovalRef: "fixture-approval", BackupRef: "fixture-backup", StoppedRuntimesRef: "fixture-no-processes", RequestID: uuid.MustParse("00000000-0000-4000-8000-000000000001")},
		Artifacts: []persistence.Artifact{{Identity: base, ManifestSHA256: hash("manifest1")}, {Identity: persistence.ArtifactIdentity{Version: 12, Filename: "migrations/0012_synthetic.up.sql", SHA256: hash("feature12")}, ManifestSHA256: hash("manifest12"), Prerequisites: []persistence.ArtifactIdentity{base}, Feature: &f}},
		Features:  []persistence.Feature{{Identity: f, Tables: []persistence.TableGrant{{Schema: "synthetic_feature", Table: "items", Select: true, InsertColumns: []string{"id", "body"}, UpdateColumns: []string{"body"}}}}},
		Shared:    persistence.Shared{Mode: "legacy", Artifacts: []persistence.SharedArtifact{{Revision: 1, Filename: "pkg/eventstore/schema.sql", SHA256: hash("shared")}}}}
}

func TestProfileRejectsIncompleteAndContradictoryIdentity(t *testing.T) {
	cases := map[string]func(*persistence.Specification){
		"owner": func(s *persistence.Specification) { s.Owner = "foreign" }, "database": func(s *persistence.Specification) { s.Database = "justix_inventory" }, "role": func(s *persistence.Specification) { s.RuntimeRole = s.Database },
		"format": func(s *persistence.Specification) { s.FormatRevision = 0 }, "history": func(s *persistence.Specification) { s.HistorySHA256 = persistence.Digest{} }, "history revision": func(s *persistence.Specification) { s.HistoryRevision = 2 },
		"nil artifacts": func(s *persistence.Specification) { s.Artifacts = nil }, "head": func(s *persistence.Specification) { s.Head = 17 }, "zero version": func(s *persistence.Specification) { s.Artifacts[0].Identity.Version = 0 },
		"duplicate version": func(s *persistence.Specification) { s.Artifacts[1].Identity.Version = 1; s.Head = 1 }, "reordered": func(s *persistence.Specification) { s.Artifacts[0], s.Artifacts[1] = s.Artifacts[1], s.Artifacts[0] },
		"duplicate file": func(s *persistence.Specification) {
			s.Artifacts[1].Identity.Filename = s.Artifacts[0].Identity.Filename
		}, "escape": func(s *persistence.Specification) {
			s.Artifacts[1].Identity.Filename = "migrations/../0012_synthetic.up.sql"
		},
		"sql identifier injection": func(s *persistence.Specification) { s.Features[0].Tables[0].Schema = "x; SELECT 1" }, "zero sql": func(s *persistence.Specification) { s.Artifacts[1].Identity.SHA256 = persistence.Digest{} }, "zero manifest": func(s *persistence.Specification) { s.Artifacts[1].ManifestSHA256 = persistence.Digest{} },
		"missing predecessor": func(s *persistence.Specification) { s.Artifacts[1].Prerequisites = nil }, "wrong prerequisite": func(s *persistence.Specification) { s.Artifacts[1].Prerequisites[0].SHA256 = hash("wrong") }, "duplicate prerequisite": func(s *persistence.Specification) {
			s.Artifacts[1].Prerequisites = append(s.Artifacts[1].Prerequisites, s.Artifacts[1].Prerequisites[0])
		},
		"baseline feature": func(s *persistence.Specification) { s.Artifacts[0].Feature = s.Artifacts[1].Feature }, "missing feature": func(s *persistence.Specification) { s.Artifacts[1].Feature = nil }, "missing feature set": func(s *persistence.Specification) { s.Features = nil }, "duplicate feature": func(s *persistence.Specification) { s.Features = append(s.Features, s.Features[0]) },
		"metadata grant": func(s *persistence.Specification) { s.Features[0].Tables[0].Schema = "owner_migrations" }, "shared grant": func(s *persistence.Specification) { s.Features[0].Tables[0].Schema = "eventstore" }, "duplicate object": func(s *persistence.Specification) {
			s.Features[0].Tables = append(s.Features[0].Tables, s.Features[0].Tables[0])
		},
		"contradictory insert": func(s *persistence.Specification) { s.Features[0].Tables[0].Insert = true }, "duplicate column": func(s *persistence.Specification) { s.Features[0].Tables[0].UpdateColumns = []string{"body", "body"} }, "empty grant": func(s *persistence.Specification) {
			s.Features[0].Tables[0] = persistence.TableGrant{Schema: "synthetic_feature", Table: "items"}
		},
		"missing baseline": func(s *persistence.Specification) { s.Baseline.RequestID = uuid.Nil }, "blank ref": func(s *persistence.Specification) { s.Baseline.EvidenceRef = " " }, "control ref": func(s *persistence.Specification) { s.Baseline.EvidenceRef = "x\n" }, "oversize ref": func(s *persistence.Specification) { s.Baseline.EvidenceRef = strings.Repeat("a", 2049) }, "invalid unicode ref": func(s *persistence.Specification) { s.Baseline.EvidenceRef = string([]byte{255}) },
		"unknown mode": func(s *persistence.Specification) { s.Shared.Mode = "auto" }, "missing shared": func(s *persistence.Specification) { s.Shared.Artifacts = nil }, "shared collision": func(s *persistence.Specification) { s.Shared.Artifacts[0].Filename = s.Artifacts[0].Identity.Filename }, "shared gap": func(s *persistence.Specification) { s.Shared.Artifacts[0].Revision = 2 }, "shared escape": func(s *persistence.Specification) { s.Shared.Artifacts[0].Filename = "../schema.sql" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := spec()
			mutate(&s)
			if _, err := persistence.NewProfile(s); err == nil {
				t.Fatal("invalid specification accepted")
			}
		})
	}
	var zero persistence.Profile
	if zero.Valid() || zero.Digest() != (persistence.Digest{}) || zero.VerifyFiles(t.TempDir()) == nil {
		t.Fatal("zero profile usable")
	}
}

func TestProfileDefensiveCopiesCanonicalIdentityAndConcurrentReads(t *testing.T) {
	s := spec()
	p, err := persistence.NewProfile(s)
	if err != nil {
		t.Fatal(err)
	}
	original := p.Specification()
	digest := p.Digest()
	s.Artifacts[0].Identity.SHA256 = hash("mutated")
	s.Artifacts[1].Prerequisites[0].Filename = "mutated"
	s.Artifacts[1].Feature.ID = "mutated"
	s.Features[0].Tables[0].UpdateColumns[0] = "immutable"
	s.Shared.Artifacts[0].SHA256 = hash("mutated")
	if !reflect.DeepEqual(original, p.Specification()) || p.Digest() != digest {
		t.Fatal("input mutation changed sealed profile")
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 30 {
				copy := p.Specification()
				copy.Artifacts[1].Feature.ID = "mutated"
				copy.Features[0].Tables[0].InsertColumns[0] = "mutated"
				if p.Digest() != digest || !reflect.DeepEqual(p.Specification(), original) {
					t.Error("copy mutation or race")
				}
			}
		})
	}
	wg.Wait()
	s = original
	s.Features[0].Tables[0].InsertColumns = []string{"id", "body"}
	a, _ := persistence.NewProfile(s)
	s.Features[0].Tables[0].InsertColumns = []string{"body", "id"}
	b, _ := persistence.NewProfile(s)
	if a.Digest() != b.Digest() {
		t.Fatal("set order affects canonical identity")
	}
	s.Baseline.BackupRef = "other-reviewed-backup"
	c, _ := persistence.NewProfile(s)
	if c.Digest() == b.Digest() {
		t.Fatal("baseline not bound")
	}
}

func TestVerifyAllHistoricalFilesAndRejectSymlinks(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "migrations"), 0700); err != nil {
		t.Fatal(err)
	}
	s := spec()
	for i, a := range s.Artifacts {
		body := "base"
		if a.Identity.Version == 12 {
			body = "feature12"
		}
		if err := os.WriteFile(filepath.Join(root, a.Identity.Filename), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		manifest, err := json.Marshal(persistence.ArtifactManifest{FormatRevision: s.FormatRevision, Owner: s.Owner, Identity: a.Identity, Prerequisites: a.Prerequisites, Feature: a.Feature})
		if err != nil {
			t.Fatal(err)
		}
		s.Artifacts[i].ManifestSHA256 = hash(string(manifest))
		if err := os.WriteFile(filepath.Join(root, strings.TrimSuffix(a.Identity.Filename, ".up.sql")+".manifest.json"), manifest, 0600); err != nil {
			t.Fatal(err)
		}
	}
	p, _ := persistence.NewProfile(s)
	if err := p.VerifyFiles(root); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, s.Artifacts[0].Identity.Filename)
	if err := os.WriteFile(base, []byte("changed retained baseline"), 0600); err != nil {
		t.Fatal(err)
	}
	if p.VerifyFiles(root) == nil {
		t.Fatal("historical hash drift accepted")
	}
	if err := os.Remove(base); err != nil {
		t.Fatal(err)
	}
	if p.VerifyFiles(root) == nil {
		t.Fatal("missing historical file accepted")
	}
	target := filepath.Join(root, "original")
	if err := os.WriteFile(target, []byte("base"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, base); err != nil {
		t.Fatal(err)
	}
	if p.VerifyFiles(root) == nil {
		t.Fatal("valid-byte symlink accepted")
	}
	if err := os.Remove(base); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "absent"), base); err != nil {
		t.Fatal(err)
	}
	if p.VerifyFiles(root) == nil {
		t.Fatal("dangling symlink accepted")
	}
	if err := os.Remove(base); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base, []byte("base"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "migrations"), filepath.Join(root, "actual")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "actual"), filepath.Join(root, "migrations")); err != nil {
		t.Fatal(err)
	}
	if p.VerifyFiles(root) == nil {
		t.Fatal("directory symlink accepted")
	}
	if p.VerifyFiles("relative") == nil {
		t.Fatal("relative trust root accepted")
	}
}

func TestAdjacentManifestBytesAndTypedIdentityAreBothRequired(t *testing.T) {
	for _, scenario := range []string{"historical manifest bytes", "missing historical manifest", "manifest symlink", "dangling manifest", "wrong owner with matching hash", "wrong predecessor with matching hash", "unknown field with matching hash", "trailing JSON with matching hash"} {
		t.Run(scenario, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(root, "migrations"), 0700); err != nil {
				t.Fatal(err)
			}
			s := spec()
			for i, a := range s.Artifacts {
				body := "base"
				if i == 1 {
					body = "feature12"
				}
				if err := os.WriteFile(filepath.Join(root, a.Identity.Filename), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
				m := persistence.ArtifactManifest{FormatRevision: 1, Owner: s.Owner, Identity: a.Identity, Prerequisites: a.Prerequisites, Feature: a.Feature}
				if i == 0 && scenario == "wrong owner with matching hash" {
					m.Owner = "inventory"
				}
				if i == 1 && scenario == "wrong predecessor with matching hash" {
					m.Prerequisites = []persistence.ArtifactIdentity{}
				}
				b, err := json.Marshal(m)
				if err != nil {
					t.Fatal(err)
				}
				if i == 0 && scenario == "unknown field with matching hash" {
					b = append([]byte(`{"unknown":true,`), b[1:]...)
				}
				if i == 0 && scenario == "trailing JSON with matching hash" {
					b = append(b, []byte(` {}`)...)
				}
				s.Artifacts[i].ManifestSHA256 = hash(string(b))
				manifestPath := filepath.Join(root, strings.TrimSuffix(a.Identity.Filename, ".up.sql")+".manifest.json")
				if err := os.WriteFile(manifestPath, b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			p := profile(t, s)
			base := filepath.Join(root, "migrations/0001_mechanics.manifest.json")
			switch scenario {
			case "historical manifest bytes":
				if err := os.WriteFile(base, []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing historical manifest":
				if err := os.Remove(base); err != nil {
					t.Fatal(err)
				}
			case "manifest symlink", "dangling manifest":
				target := filepath.Join(root, "saved-manifest")
				if err := os.Rename(base, target); err != nil {
					t.Fatal(err)
				}
				if scenario == "dangling manifest" {
					target += "-absent"
				}
				if err := os.Symlink(target, base); err != nil {
					t.Fatal(err)
				}
			}
			if p.VerifyFiles(root) == nil {
				t.Fatal("invalid adjacent manifest accepted")
			}
		})
	}
}

func TestProfileRejectsDuplicateFeatureKeysAndFilenameVersionMismatch(t *testing.T) {
	s := spec()
	s.Artifacts[1].Identity.Filename = "migrations/0017_synthetic.up.sql"
	if _, err := persistence.NewProfile(s); err == nil {
		t.Fatal("filename version mismatch accepted")
	}
	s = spec()
	f := *s.Artifacts[1].Feature
	f.SHA256 = hash("different bytes same feature key")
	s.Artifacts = append(s.Artifacts, persistence.Artifact{Identity: persistence.ArtifactIdentity{Version: 17, Filename: "migrations/0017_second.up.sql", SHA256: hash("second")}, ManifestSHA256: hash("manifest17"), Prerequisites: []persistence.ArtifactIdentity{s.Artifacts[1].Identity}, Feature: &f})
	s.Head = 17
	s.Features = append(s.Features, persistence.Feature{Identity: f, Tables: []persistence.TableGrant{{Schema: "another", Table: "items", Select: true}}})
	if _, err := persistence.NewProfile(s); err == nil {
		t.Fatal("same marker key with divergent hash accepted")
	}
}

func TestProfileRejectsMarkerOnlyAndOverlappingFeatureGrants(t *testing.T) {
	s := spec()
	s.Features[0].Tables = nil
	if _, err := persistence.NewProfile(s); err == nil {
		t.Fatal("marker-only feature selected an implicit privilege policy")
	}
	s = spec()
	f := persistence.FeatureIdentity{ID: "synthetic.later", Revision: 1, SHA256: hash("later contract")}
	s.Artifacts = append(s.Artifacts, persistence.Artifact{Identity: persistence.ArtifactIdentity{Version: 17, Filename: "migrations/0017_later.up.sql", SHA256: hash("later")}, ManifestSHA256: hash("manifest17"), Prerequisites: []persistence.ArtifactIdentity{s.Artifacts[1].Identity}, Feature: &f})
	s.Head = 17
	s.Features = append(s.Features, persistence.Feature{Identity: f, Tables: s.Features[0].Tables})
	if _, err := persistence.NewProfile(s); err == nil {
		t.Fatal("overlapping distinct feature contracts were implicitly unioned or superseded")
	}
}
