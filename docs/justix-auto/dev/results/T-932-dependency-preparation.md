# T-932 coordinator dependency preparation

2026-09-16. Isolated task/T-932-owner-migration-driver at base
`e4833a0b606377a5ecdd25f9743e6b9dc5a783fe`; own-project Git gate passed.
The coordinator adds only the already approved owner-migration contract pin
`github.com/golang-migrate/migrate/v4 v4.20.1` and its two checksums.

- Module: `h1:2N/ToVTKrKl58ynBpgeVJ4In7VcLCjWTZtm4eP1LxhU=`.
- go.mod: `h1:DDPgKVb4ovSWc4FwSPfV2Uz1160f4XBiTHTrAJtljmM=`.
- Audited upstream origin: `504568a3cbd23b8754760f55a3d89aec1b0c4963`, tag v4.20.1.

The exact cached archive came from the previously reviewed
`/private/tmp/justixauto-owner-migration-audit/cache/download` through a local
file GOPROXY. Go verified the sums while populating the ordinary task module
cache. No network download or dependency resolution update was needed for this
step. All previous go.mod/go.sum lines and the pgx/v5 v5.11.0 pin are preserved.
The two added sums were placed in normal sorted order before commit.

Pinned Go1.27.1 commands completed with exit0:

```sh
bash tools/check-git.sh
GOPROXY=file:///private/tmp/justixauto-owner-migration-audit/cache/download bash tools/go.sh mod download -json github.com/golang-migrate/migrate/v4@v4.20.1
GOPROXY=off bash tools/go.sh list -mod=readonly -deps github.com/golang-migrate/migrate/v4
GOPROXY=off bash tools/go.sh build -mod=readonly github.com/golang-migrate/migrate/v4
git diff --check
```

Commands used GOMODCACHE=/private/tmp/justixauto-t003-modcache and
GOCACHE=/private/tmp/justixauto-integration-gocache. The selected migrate core
package dependency list contains standard library and the migrate module's own
database/source/internal-url packages only. No upstream PostgreSQL/libpq driver
is imported. The future project adapter must implement the actual public Driver
interface using the existing native pgx connection, per the approved contract.
This check does not claim every module in upstream's optional-driver graph was
downloaded or tested, or that the project migration driver exists yet.

Implementation owns only driver.go/driver_test.go and its result. Root alone
owns these dependency changes. Actual-engine/fault tests and independent review
of the combined exact task commit remain required before integration. Existing
SQL/default provisioning and trusted adjacent manifest gates remain unchanged.
