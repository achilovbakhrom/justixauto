// Package persistence validates trusted, owner-local installation evidence.
// A profile is release configuration, never request data or business authority.
package persistence

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

var ErrInvalidProfile = errors.New("invalid exact persistence profile")
var ErrIncompatible = errors.New("persistence is incompatible")
var ErrUnsupported = errors.New("unsupported persistence profile")

type Digest [32]byte

// ArtifactIdentity identifies original SQL bytes, not an installation attempt.
type ArtifactIdentity struct {
	Version  int64
	Filename string
	SHA256   Digest
}

type FeatureIdentity struct {
	ID       string
	Revision uint32
	SHA256   Digest
}

type Artifact struct {
	Identity       ArtifactIdentity
	ManifestSHA256 Digest
	Prerequisites  []ArtifactIdentity
	Feature        *FeatureIdentity
}

// ArtifactManifest is the adjacent producer-owned .manifest.json format. It
// excludes its own digest and any installing profile/request identity. The
// release profile hashes ORIGINAL manifest bytes and binds this complete value.
type ArtifactManifest struct {
	FormatRevision uint32             `json:"format_revision"`
	Owner          string             `json:"owner"`
	Identity       ArtifactIdentity   `json:"artifact"`
	Prerequisites  []ArtifactIdentity `json:"prerequisites"`
	Feature        *FeatureIdentity   `json:"feature_contract"`
}

// TableGrant is an exact required AND maximum permission set. Column grants
// cannot be combined with the corresponding broad table grant. No TRUNCATE,
// REFERENCES, TRIGGER, MAINTAIN, delegation, functions or sequences are exposed.
type TableGrant struct {
	Schema        string
	Table         string
	Select        bool
	Insert        bool
	Update        bool
	Delete        bool
	SelectColumns []string
	InsertColumns []string
	UpdateColumns []string
}

type Feature struct {
	Identity FeatureIdentity
	Tables   []TableGrant
}

// Baseline is the exact accepted evidence identity. Construction validates
// shape, not truth: only the outer deployment authority can accept attestation.
type Baseline struct {
	Kind               string
	EvidenceRef        string
	ApprovalRef        string
	BackupRef          string
	StoppedRuntimesRef string
	RequestID          uuid.UUID
}

type SharedArtifact struct {
	Revision uint32
	Filename string
	SHA256   Digest
}

type Shared struct {
	Mode      string
	Artifacts []SharedArtifact
}

type Specification struct {
	Owner           string
	Database        string
	RuntimeRole     string
	FormatRevision  uint32
	Head            int64
	Artifacts       []Artifact
	HistoryRevision uint32
	HistorySHA256   Digest
	Baseline        Baseline
	Features        []Feature
	Shared          Shared
}

// Profile has no exported mutable state. The zero value is invalid. Every
// returned specification is a deep copy; sharing a Profile is safe.
type Profile struct{ data *profileData }
type profileData struct {
	spec   Specification
	digest Digest
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
var artifactPath = regexp.MustCompile(`^migrations/[0-9]+_[a-z][a-z0-9_]*[.]up[.]sql$`)
var featureID = regexp.MustCompile(`^[a-z][a-z0-9.-]{0,127}$`)

// NewProfile is inert. Filenames here are identifiers, never SQL. The runner
// must additionally call VerifyFiles on its trusted owner root before execution.
func NewProfile(input Specification) (Profile, error) {
	s := clone(input)
	bad := func(reason string) (Profile, error) {
		return Profile{}, fmt.Errorf("%w: %s", ErrInvalidProfile, reason)
	}
	if !slices.Contains([]string{"identity", "inventory", "commerce", "retail", "financing", "insurance", "documents"}, s.Owner) ||
		s.Database != "justix_"+s.Owner || s.RuntimeRole != s.Database+"_runtime" || s.FormatRevision != 1 || s.HistoryRevision != 1 || s.HistorySHA256 == (Digest{}) {
		return bad("owner or history identity")
	}
	if s.Shared.Mode != "legacy" && s.Shared.Mode != "custody" {
		return bad("mode")
	}
	if len(s.Artifacts) == 0 || s.Head < 1 || s.Artifacts[0].Identity.Version != 1 || s.Artifacts[0].Identity.Filename != "migrations/0001_mechanics.up.sql" || s.Head != s.Artifacts[len(s.Artifacts)-1].Identity.Version {
		return bad("complete private head")
	}
	if !slices.Contains([]string{"verified-installation", "attested-existing-v1"}, s.Baseline.Kind) || s.Baseline.RequestID == uuid.Nil {
		return bad("baseline identity")
	}
	for _, ref := range []string{s.Baseline.EvidenceRef, s.Baseline.ApprovalRef, s.Baseline.BackupRef, s.Baseline.StoppedRuntimesRef} {
		if !validRef(ref) {
			return bad("baseline reference")
		}
	}
	seen := map[int64]ArtifactIdentity{}
	paths := map[string]bool{}
	features := map[FeatureIdentity]bool{}
	featureKeys := map[string]bool{}
	last := int64(0)
	for i, a := range s.Artifacts {
		id := a.Identity
		if id.Version <= last || id.SHA256 == (Digest{}) || a.ManifestSHA256 == (Digest{}) || !artifactPath.MatchString(id.Filename) || path.Clean(id.Filename) != id.Filename || paths[id.Filename] {
			return bad("private artifact")
		}
		fileVersion, err := strconv.ParseInt(strings.Split(strings.TrimPrefix(id.Filename, "migrations/"), "_")[0], 10, 64)
		if err != nil || fileVersion != id.Version {
			return bad("filename version")
		}
		paths[id.Filename] = true
		prereqs := map[int64]bool{}
		for _, pre := range a.Prerequisites {
			prior, ok := seen[pre.Version]
			if !ok || prior != pre || prereqs[pre.Version] {
				return bad("prerequisite")
			}
			prereqs[pre.Version] = true
		}
		if i == 0 {
			if a.Feature != nil || len(a.Prerequisites) != 0 {
				return bad("baseline feature")
			}
		} else {
			if !prereqs[last] || a.Feature == nil || !validFeature(*a.Feature) || features[*a.Feature] {
				return bad("feature or predecessor")
			}
			key := fmt.Sprintf("%s/%d", a.Feature.ID, a.Feature.Revision)
			if featureKeys[key] {
				return bad("duplicate feature marker key")
			}
			featureKeys[key] = true
			features[*a.Feature] = true
		}
		last = id.Version
		seen[id.Version] = id
	}
	objects := map[string]bool{}
	declared := map[FeatureIdentity]bool{}
	for _, f := range s.Features {
		if !features[f.Identity] || declared[f.Identity] || len(f.Tables) == 0 {
			return bad("feature contract set")
		}
		declared[f.Identity] = true
		for _, g := range f.Tables {
			key := g.Schema + "." + g.Table
			if !identifier.MatchString(g.Schema) || !identifier.MatchString(g.Table) || strings.HasPrefix(g.Schema, "pg_") || slices.Contains([]string{"eventstore", "owner_migrations", "public", "information_schema", s.Owner + "_mechanics"}, g.Schema) || objects[key] {
				return bad("feature object or namespace")
			}
			objects[key] = true
			if !(g.Select || g.Insert || g.Update || g.Delete || len(g.SelectColumns)+len(g.InsertColumns)+len(g.UpdateColumns) > 0) {
				return bad("empty grant")
			}
			for _, pair := range []struct {
				broad   bool
				columns []string
			}{{g.Select, g.SelectColumns}, {g.Insert, g.InsertColumns}, {g.Update, g.UpdateColumns}} {
				if pair.broad && len(pair.columns) > 0 {
					return bad("contradictory grants")
				}
				cols := map[string]bool{}
				for _, col := range pair.columns {
					if !identifier.MatchString(col) || cols[col] {
						return bad("column grant")
					}
					cols[col] = true
				}
			}
		}
	}
	if len(declared) != len(features) {
		return bad("missing feature contract")
	}
	if len(s.Shared.Artifacts) == 0 {
		return bad("missing shared base")
	}
	for i, a := range s.Shared.Artifacts {
		if a.Revision != uint32(i+1) || a.SHA256 == (Digest{}) || !utf8.ValidString(a.Filename) || strings.ContainsFunc(a.Filename, unicode.IsControl) || a.Filename == "" || path.Clean(a.Filename) != a.Filename || strings.Contains(a.Filename, "\\") || strings.HasPrefix(a.Filename, "/") || strings.HasPrefix(a.Filename, "../") || paths[a.Filename] {
			return bad("shared artifact lineage or namespace")
		}
		paths[a.Filename] = true
	}
	// Canonical order for sets; private/shared artifact order remains semantic.
	if s.Features == nil {
		s.Features = []Feature{}
	}
	for i := range s.Artifacts {
		if s.Artifacts[i].Prerequisites == nil {
			s.Artifacts[i].Prerequisites = []ArtifactIdentity{}
		}
		slices.SortFunc(s.Artifacts[i].Prerequisites, func(a, b ArtifactIdentity) int {
			if a.Version < b.Version {
				return -1
			}
			if a.Version > b.Version {
				return 1
			}
			return 0
		})
	}
	for i := range s.Features {
		for j := range s.Features[i].Tables {
			g := &s.Features[i].Tables[j]
			for _, cols := range []*[]string{&g.SelectColumns, &g.InsertColumns, &g.UpdateColumns} {
				if *cols == nil {
					*cols = []string{}
				}
			}
			slices.Sort(g.SelectColumns)
			slices.Sort(g.InsertColumns)
			slices.Sort(g.UpdateColumns)
		}
		slices.SortFunc(s.Features[i].Tables, func(a, b TableGrant) int { return strings.Compare(a.Schema+"."+a.Table, b.Schema+"."+b.Table) })
	}
	slices.SortFunc(s.Features, func(a, b Feature) int {
		if c := strings.Compare(a.Identity.ID, b.Identity.ID); c != 0 {
			return c
		}
		if a.Identity.Revision < b.Identity.Revision {
			return -1
		}
		if a.Identity.Revision > b.Identity.Revision {
			return 1
		}
		return 0
	})
	encoded, err := json.Marshal(s)
	if err != nil {
		return bad("canonical encoding")
	}
	return Profile{&profileData{s, Digest(sha256.Sum256(encoded))}}, nil
}

func validFeature(f FeatureIdentity) bool {
	return featureID.MatchString(f.ID) && f.Revision > 0 && f.SHA256 != (Digest{})
}
func validRef(s string) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 2048 && !strings.ContainsFunc(s, unicode.IsControl)
}
func clone(s Specification) Specification {
	out := s
	out.Artifacts = slices.Clone(s.Artifacts)
	for i := range out.Artifacts {
		out.Artifacts[i].Prerequisites = slices.Clone(s.Artifacts[i].Prerequisites)
		if s.Artifacts[i].Feature != nil {
			f := *s.Artifacts[i].Feature
			out.Artifacts[i].Feature = &f
		}
	}
	out.Features = slices.Clone(s.Features)
	for i := range out.Features {
		out.Features[i].Tables = slices.Clone(s.Features[i].Tables)
		for j := range out.Features[i].Tables {
			g := &out.Features[i].Tables[j]
			g.SelectColumns = slices.Clone(g.SelectColumns)
			g.InsertColumns = slices.Clone(g.InsertColumns)
			g.UpdateColumns = slices.Clone(g.UpdateColumns)
		}
	}
	out.Shared.Artifacts = slices.Clone(s.Shared.Artifacts)
	return out
}
func (p Profile) Valid() bool { return p.data != nil }
func (p Profile) Digest() Digest {
	if p.data == nil {
		return Digest{}
	}
	return p.data.digest
}
func (p Profile) Specification() Specification {
	if p.data == nil {
		return Specification{}
	}
	return clone(p.data.spec)
}

// VerifyFiles checks every private artifact, including historical files, under
// an explicit trusted owner directory. It rejects symlinks in every component
// and verifies original bytes. This is not hostile concurrent filesystem fencing;
// the runner must retain verified bytes and rehash after its reader boundary.
// Shared/template bytes are separately verified by their explicit outer runner.
func (p Profile) VerifyFiles(ownerRoot string) error {
	if p.data == nil || !filepath.IsAbs(ownerRoot) {
		return ErrInvalidProfile
	}
	root := filepath.Clean(ownerRoot)
	for current := root; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%w: unsafe owner root", ErrInvalidProfile)
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	for _, a := range p.data.spec.Artifacts {
		for _, name := range []string{a.Identity.Filename, strings.TrimSuffix(a.Identity.Filename, ".up.sql") + ".manifest.json"} {
			current := root
			parts := strings.Split(name, "/")
			for i, part := range parts {
				current = filepath.Join(current, part)
				info, err := os.Lstat(current)
				if err != nil || info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) || (i == len(parts)-1 && !info.Mode().IsRegular()) {
					return fmt.Errorf("%w: unsafe artifact path", ErrInvalidProfile)
				}
			}
			b, err := os.ReadFile(current)
			expected := a.Identity.SHA256
			if name != a.Identity.Filename {
				expected = a.ManifestSHA256
			}
			if err != nil || Digest(sha256.Sum256(b)) != expected {
				return fmt.Errorf("%w: artifact bytes", ErrInvalidProfile)
			}
			if name != a.Identity.Filename {
				if err := validateManifestJSON(b); err != nil {
					return err
				}
				var manifest ArtifactManifest
				decoder := json.NewDecoder(bytes.NewReader(b))
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(&manifest); err != nil {
					return fmt.Errorf("%w: manifest format", ErrInvalidProfile)
				}
				var extra any
				if decoder.Decode(&extra) != io.EOF {
					return fmt.Errorf("%w: manifest trailing data", ErrInvalidProfile)
				}
				if manifest.FormatRevision != p.data.spec.FormatRevision || manifest.Owner != p.data.spec.Owner || manifest.Identity != a.Identity || !slices.Equal(manifest.Prerequisites, a.Prerequisites) || !reflect.DeepEqual(manifest.Feature, a.Feature) {
					return fmt.Errorf("%w: manifest identity", ErrInvalidProfile)
				}
			}
		}
	}
	return nil
}

// Validate original bytes before encoding/json can replace malformed Unicode,
// overwrite duplicate members, or match a differently-cased typed field name.
// Object names compare decoded codepoints; no Unicode normalization is applied.
func validateManifestJSON(b []byte) error {
	bad := func(reason string) error { return fmt.Errorf("%w: manifest %s", ErrInvalidProfile, reason) }
	if !utf8.Valid(b) || !json.Valid(b) {
		return bad("JSON encoding")
	}
	for i := 0; i < len(b); i++ {
		if b[i] != '"' {
			continue
		}
		i++
		for i < len(b) && b[i] != '"' {
			if b[i] != '\\' {
				i++
				continue
			}
			if b[i+1] != 'u' {
				i += 2
				continue
			}
			u, _ := strconv.ParseUint(string(b[i+2:i+6]), 16, 16)
			if u >= 0xdc00 && u <= 0xdfff {
				return bad("Unicode surrogate")
			}
			if u >= 0xd800 && u <= 0xdbff {
				if i+12 > len(b) || b[i+6] != '\\' || b[i+7] != 'u' {
					return bad("Unicode surrogate")
				}
				low, err := strconv.ParseUint(string(b[i+8:i+12]), 16, 16)
				if err != nil || low < 0xdc00 || low > 0xdfff {
					return bad("Unicode surrogate")
				}
				i += 6
			}
			i += 6
		}
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var value func() (any, error)
	value = func() (any, error) {
		token, err := d.Token()
		if err != nil {
			return nil, bad("JSON token")
		}
		switch token {
		case json.Delim('{'):
			out := map[string]any{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return nil, bad("JSON member")
				}
				name, ok := key.(string)
				if !ok {
					return nil, bad("JSON member")
				}
				if _, exists := out[name]; exists {
					return nil, bad("duplicate member")
				}
				v, err := value()
				if err != nil {
					return nil, err
				}
				out[name] = v
			}
			if _, err := d.Token(); err != nil {
				return nil, bad("JSON object")
			}
			return out, nil
		case json.Delim('['):
			out := []any{}
			for d.More() {
				v, err := value()
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			}
			if _, err := d.Token(); err != nil {
				return nil, bad("JSON array")
			}
			return out, nil
		default:
			return token, nil
		}
	}
	root, err := value()
	if err != nil {
		return err
	}
	exact := func(v any, keys ...string) (map[string]any, bool) {
		m, ok := v.(map[string]any)
		if !ok || len(m) != len(keys) {
			return nil, false
		}
		for _, key := range keys {
			if _, ok := m[key]; !ok {
				return nil, false
			}
		}
		return m, true
	}
	m, ok := exact(root, "format_revision", "owner", "artifact", "prerequisites", "feature_contract")
	if !ok {
		return bad("field names")
	}
	// encoding/json's fixed-array decoder truncates extra values, zero-fills
	// short arrays and ignores null numeric elements. Validate every raw scalar
	// and every digest array here, before any typed conversion can discard them.
	integer := func(v any, bits int, positive bool) bool {
		n, ok := v.(json.Number)
		if !ok {
			return false
		}
		u, err := strconv.ParseUint(string(n), 10, bits)
		return err == nil && (!positive || u > 0)
	}
	identity := func(v any, feature bool) error {
		version, name := "Version", "Filename"
		bits := 63
		if feature {
			version, name = "Revision", "ID"
			bits = 32
		}
		item, ok := exact(v, version, name, "SHA256")
		if !ok {
			return bad("field names")
		}
		if _, ok := item[name].(string); !ok || !integer(item[version], bits, true) {
			return bad("identity scalar")
		}
		digest, ok := item["SHA256"].([]any)
		if !ok || len(digest) != 32 {
			return bad("digest shape")
		}
		for _, part := range digest {
			if !integer(part, 8, false) {
				return bad("digest byte")
			}
		}
		return nil
	}
	if _, ok := m["owner"].(string); !ok || !integer(m["format_revision"], 32, true) {
		return bad("identity scalar")
	}
	if err := identity(m["artifact"], false); err != nil {
		return err
	}
	if m["prerequisites"] != nil {
		items, ok := m["prerequisites"].([]any)
		if !ok {
			return bad("prerequisite shape")
		}
		for _, item := range items {
			if err := identity(item, false); err != nil {
				return err
			}
		}
	}
	if m["feature_contract"] != nil {
		if err := identity(m["feature_contract"], true); err != nil {
			return err
		}
	}
	return nil
}
