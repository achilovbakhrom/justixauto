# Dependency lock — ADR-13 baseline

Status: T-001 baseline approved; coordinator-approved T-004 Vitest correction below awaits T-004 exact-commit QA.
Prepared 2026-09-14; public evidence retrieved on that UTC date.
Task branch: `task/T-001-version-lock`; base `17f2c2cefdeb326a14cf93b71c19495c10234750`.
Original [T-001](tasks/T-001.md) pins were approved in [the coordinator record](../state/dependency-lock-approval.md). The [T-004 correction](../state/t004-vitest-correction-approval.md) replaces only the Vitest/coverage pair after actual compiler checks. Initial metadata-only evidence below remains scoped to T-001.

## Authority and scope

Confirmed: [architecture](../architecture.md) ADR-11/12/13 selects React/TypeScript/Vite, Router, TanStack Query, RHF/Zod, CSS Modules/Radix Dialog; Echo/GORM, maintained AMQP client, Zap/OTel and golang-migrate. Seven independent Go services own separate stores; one Go module and npm workspaces share technical tooling. This proposal fills exact versions within those families.

Observed: the [original Gaze audit](../reference/gaze-reference.md) and [CC audit](../reference/gaze-executor-cc-reference.md) demonstrate layering, composition roots, command/replay and subscribers. Neither proves durable SQL-to-broker delivery; CC publishes before commit. Their historical pins, snapshot behavior, trading packages and credentials are not this lock. HTML/browser arithmetic and demo accounts provide no financial/security authority.

Proposed: the following reproducible development baseline. No packages, compilers, containers or browser binaries were installed or executed. No canonical board, architecture, contract or application files were changed. Read-only registry queries used public names/versions only.

## Runtime and support decision

| Component | Proposed exact version | Dated official evidence and consequence |
|---|---|---|
| Go compiler | 1.27.1 | [Release history](https://go.dev/doc/devel/release), released 2026-09-01; 1.27 and 1.26 are the two supported series. Installed 1.24.2 is outside support. Pin module `go 1.27.1`; enforce compiler equality through `tools/go.sh`, with `GOTOOLCHAIN=local` to prevent an unreviewed download. |
| Node | 24.21.0 LTS | [Release](https://nodejs.org/en/blog/release/v24.21.0) dated 2026-09-08; [distribution index](https://nodejs.org/dist/index.json) records 2026-09-07 and bundled npm 11.19.0. Record this one-day source difference rather than invent a date. [Release schedule](https://github.com/nodejs/Release) lists Node 24 support through 2028-04-30. |
| npm | 11.19.0 | Bundled with the selected Node distribution; [exact registry metadata](https://registry.npmjs.org/npm/11.19.0) requires Node ^20.17.0 or >=22.9.0. No separate npm 12 upgrade is required. |
| PostgreSQL | 18.6 | [Release notes](https://www.postgresql.org/docs/release/18.6/) dated 2026-08-13 include security fixes, including role-dependent cached-plan invalidation (CVE-2026-14666). [Support table](https://www.postgresql.org/support/versioning/) gives PostgreSQL 18 support through 2030-11-14. New stores only; no existing data upgrade is authorized. |
| RabbitMQ | 4.3.5 | [Release information](https://www.rabbitmq.com/release-information), released 2026-08-17, community support through 2026-11-30. 4.2 community support ended 2026-07-31; commercial support is not presumed. Revisit before that deadline. |
| Alpine runtime/base | 3.24.1 | [Alpine release table](https://alpinelinux.org/releases/) lists 3.24 with main support through 2028-06-01; community has the shorter rolling release policy. Digest freezes bytes, not ongoing security maintenance. |
| ClamAV | 1.4.6 LTS | [Support matrix](https://docs.clamav.net/faq/faq-eol.html) lists engine patch support through 2027-08-15 and signature downloads through 2028-08-15. [Vendor security announcement](https://blog.clamav.net/) dated 2026-08-07 includes parser, overflow and STATS fixes in 1.4.6. |

All versions were actually returned by upstream APIs; none was inferred from the current date or an expected release cadence. The Go module timestamps, npm registry pins and registry manifests below are evidence of availability, not proof that all application behavior works. Package libraries without an explicit LTS promise are maintained-release selections, not contractual support guarantees.

## Toolchain archives

Official checksums were read from [Go download metadata](https://go.dev/dl/?mode=json) and [Node SHASUMS256.txt](https://nodejs.org/dist/v24.21.0/SHASUMS256.txt). These checksums identify downloads; archives were not downloaded during this task. The coordinator may install into the ignored project-local `docs/justix-auto/dev/local/toolchains` path without changing global PATH.

| Download | SHA-256 |
|---|---|
| [go1.27.1.darwin-arm64.tar.gz](https://go.dev/dl/go1.27.1.darwin-arm64.tar.gz) | `ee215d57e0ec269c60cc9ceca68e6bda321ba9ee5afe24f4b0988703c2d87d12` |
| [go1.27.1.linux-amd64.tar.gz](https://go.dev/dl/go1.27.1.linux-amd64.tar.gz) | `63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445` |
| [go1.27.1.linux-arm64.tar.gz](https://go.dev/dl/go1.27.1.linux-arm64.tar.gz) | `3450b45a3f9ee8568792736a5c5e70a1f2e9b36c35a8f74958c03e51d7d92bec` |
| [node-v24.21.0-darwin-arm64.tar.gz](https://nodejs.org/dist/v24.21.0/node-v24.21.0-darwin-arm64.tar.gz) | `bed7eea5325e1108f32ce5228ddd6a5f0f08a499ee42aa7442aea583702f6057` |
| [node-v24.21.0-linux-arm64.tar.xz](https://nodejs.org/dist/v24.21.0/node-v24.21.0-linux-arm64.tar.xz) | `6ad1325edbdb5649c379b75a237147a666c95d4f9ae8d340fef2d1575d289ad2` |
| [node-v24.21.0-linux-x64.tar.xz](https://nodejs.org/dist/v24.21.0/node-v24.21.0-linux-x64.tar.xz) | `fd8e59d5a511510f6a298afb548f18c7d2b1be404d8b4a27d94fbe49f56cb2d6` |

## Go packages and verification tools

Exact module metadata and minimum Go directives were read through the official [Go module proxy protocol](https://go.dev/ref/mod#goproxy-protocol). Each source link below resolves the exact version's `.info`; replace `.info` with `.mod` to reproduce the compatibility inspection. Go 1.27.1 meets every listed minimum. Go checksums are recorded in the following section, not invented source-file hashes.

| Module | Exact version | Minimum Go | Version metadata |
|---|---|---|---|
| `github.com/labstack/echo/v4` | `v4.15.4` | 1.25.0 | [proxy](https://proxy.golang.org/github.com/labstack/echo/v4/@v/v4.15.4.info) (2026-06-15T18:23:04Z) |
| `gorm.io/gorm` | `v1.31.2` | 1.18 | [proxy](https://proxy.golang.org/gorm.io/gorm/@v/v1.31.2.info) (2026-06-22T03:37:04Z) |
| `gorm.io/driver/postgres` | `v1.6.3` | 1.25.0 | [proxy](https://proxy.golang.org/gorm.io/driver/postgres/@v/v1.6.3.info) (2026-09-14T16:04:24Z) |
| `github.com/rabbitmq/amqp091-go` | `v1.14.0` | 1.20 | [proxy](https://proxy.golang.org/github.com/rabbitmq/amqp091-go/@v/v1.14.0.info) (2026-08-18T07:07:38Z) |
| `go.uber.org/zap` | `v1.28.0` | 1.19 | [proxy](https://proxy.golang.org/go.uber.org/zap/@v/v1.28.0.info) (2026-04-28T02:13:09Z) |
| `go.opentelemetry.io/otel` | `v1.46.0` | 1.25.0 | [proxy](https://proxy.golang.org/go.opentelemetry.io/otel/@v/v1.46.0.info) (2026-08-25T16:52:31Z) |
| `go.opentelemetry.io/otel/sdk` | `v1.46.0` | 1.25.0 | [proxy](https://proxy.golang.org/go.opentelemetry.io/otel/sdk/@v/v1.46.0.info) (2026-08-25T16:52:31Z) |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` | `v1.46.0` | 1.25.0 | [proxy](https://proxy.golang.org/go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp/@v/v1.46.0.info) (2026-08-25T16:52:31Z) |
| `github.com/golang-migrate/migrate/v4` | `v4.20.1` | 1.25.11 | [proxy](https://proxy.golang.org/github.com/golang-migrate/migrate/v4/@v/v4.20.1.info) (2026-09-09T04:57:20Z) |
| `golang.org/x/crypto` | `v0.57.0` | 1.26.0 | [proxy](https://proxy.golang.org/golang.org/x/crypto/@v/v0.57.0.info) (2026-09-08T18:05:01Z) |
| `golang.org/x/vuln` | `v1.8.0` | 1.26.0 | [proxy](https://proxy.golang.org/golang.org/x/vuln/@v/v1.8.0.info) (2026-09-08T20:52:42Z) |
| `github.com/google/uuid` | `v1.6.0` | not declared | [proxy](https://proxy.golang.org/github.com/google/uuid/@v/v1.6.0.info) (2024-01-23T18:54:04Z) |
| `github.com/jackc/pgx/v5` | `v5.11.0` | 1.25.0 | [proxy](https://proxy.golang.org/github.com/jackc/pgx/v5/@v/v5.11.0.info) (2026-09-07T23:39:32Z) |
| `go.opentelemetry.io/otel/metric` | `v1.46.0` | 1.25.0 | [proxy](https://proxy.golang.org/go.opentelemetry.io/otel/metric/@v/v1.46.0.info) (2026-08-25T16:52:31Z) |
| `go.opentelemetry.io/otel/trace` | `v1.46.0` | 1.25.0 | [proxy](https://proxy.golang.org/go.opentelemetry.io/otel/trace/@v/v1.46.0.info) (2026-08-25T16:52:31Z) |
| `go.opentelemetry.io/otel/sdk/metric` | `v1.46.0` | 1.25.0 | [proxy](https://proxy.golang.org/go.opentelemetry.io/otel/sdk/metric/@v/v1.46.0.info) (2026-08-25T16:52:31Z) |

The PostgreSQL GORM adapter requires GORM 1.31.2 and pgx >=5.10.0; the selected pgx 5.11.0 satisfies that minimum. Use the migration tool's `pgx/v5` database adapter and file source only; its upstream module also lists optional drivers, which is not permission to provision their databases/cloud SDKs. Build the tool from the exact module with only the required adapter/build tags.

OTel core, trace, metric, SDK and HTTP OTLP exporter all use 1.46.0. Instrument Echo with these APIs in the technical adapter. The old `go.opentelemetry.io/contrib/.../echo/otelecho` module was inspected and explicitly declares deprecation; it is not selected. This avoids an unapproved alternate middleware family. `golang.org/x/crypto/argon2` provides Argon2id; `golang.org/x/vuln/cmd/govulncheck` is the pinned later verification tool. UUID is a technical identifier utility; it does not own domain state.

T-003 need not import every future package into an otherwise empty module just to force `go.sum` entries. When a task first uses a package, use this exact version, resolve its actual dependency graph and commit the resulting `go.mod`/`go.sum` changes in the assigned manifest task. Reject retractions, unexpected direct upgrades or a raised minimum compiler; report any conflict to the coordinator. This proposal is a direct-package lock, not a fabricated complete transitive application lock.

### Go checksum database records

Retrieved from `https://sum.golang.org/lookup/<module>@<version>` over HTTPS. Each pair identifies the module content and its go.mod. Later `go mod download/verify` must validate checksum-database signatures and content normally; no `GOSUMDB=off` or blind copying is authorized.

```text
github.com/labstack/echo/v4 v4.15.4 h1:DL45vVYa+BWE+XuW+zZNd9H0YEdZ80UAWJGcTVW4EVs=
github.com/labstack/echo/v4 v4.15.4/go.mod h1:CuMetKIRwsuO/qlAgMq+KTAalwGoB/h4tC+yPdrTj1g=
gorm.io/gorm v1.31.2 h1:3o8FXNo9v9S858gil+3LlZA1LkCOzgb4g5BL64FgaCo=
gorm.io/gorm v1.31.2/go.mod h1:XyQVbO2k6YkOis7C2437jSit3SsDK72s7n7rsSHd+Gs=
gorm.io/driver/postgres v1.6.3 h1:bAn6O2pUa8LtpWEvL5NFU4+52Tfx8Ut7IVaIacCLcI0=
gorm.io/driver/postgres v1.6.3/go.mod h1:0c4fQA44XhOklXDkgtuKqysHCycTa5i9e3EIpDGCwXk=
github.com/rabbitmq/amqp091-go v1.14.0 h1:RSaT7aOKt/OrkVUyswPDW29lnRz9psuGmfZFBmLqLek=
github.com/rabbitmq/amqp091-go v1.14.0/go.mod h1:Hy4jKW5kQART1u+JkDTF9YYOQUHXqMuhrgxOEeS7G4o=
go.uber.org/zap v1.28.0 h1:IZzaP1Fv73/T/pBMLk4VutPl36uNC+OSUh3JLG3FIjo=
go.uber.org/zap v1.28.0/go.mod h1:rDLpOi171uODNm/mxFcuYWxDsqWSAVkFdX4XojSKg/Q=
go.opentelemetry.io/otel v1.46.0 h1:FHt5/CDyVxi/8IM1CH7VE/rRgq3kLHa2mSTVMO8AWyc=
go.opentelemetry.io/otel v1.46.0/go.mod h1:Gj3SEScelsNC45tp4nSxRYlS+f5iez7W8XPMCt905kE=
go.opentelemetry.io/otel/sdk v1.46.0 h1:h5CNQQjEbuQXY/JfZtgt3i7HVFV3aHPO2OAwO2eTYPI=
go.opentelemetry.io/otel/sdk v1.46.0/go.mod h1:GAERFXFt5SYCEB+YiKUbMBeza6UaDH7GmGOZEfh2gSM=
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.46.0 h1:KrC1YrQeSt46ITMWAbgQx1M1eV1/1TKzttrBzymPmss=
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.46.0/go.mod h1:zDSEzoEqsOrgBeGvH66KRgxh90VonFyJqBHA0Pk3+rM=
github.com/golang-migrate/migrate/v4 v4.20.1 h1:2N/ToVTKrKl58ynBpgeVJ4In7VcLCjWTZtm4eP1LxhU=
github.com/golang-migrate/migrate/v4 v4.20.1/go.mod h1:DDPgKVb4ovSWc4FwSPfV2Uz1160f4XBiTHTrAJtljmM=
golang.org/x/crypto v0.57.0 h1:3ZVCjf8Ggz7zneR/EHRVx68Ctf+2pmIMP2UFhh9cC6M=
golang.org/x/crypto v0.57.0/go.mod h1:Fdz0i5U6CoizGwLda9DttjSk6qlZo25zYNtR+ycvuZA=
golang.org/x/vuln v1.8.0 h1:clG4qBU6zH5VKjti8n5j8BBuYzoSha392xXMkXS351U=
golang.org/x/vuln v1.8.0/go.mod h1:Fzm4XK3Hbl1ZvZ7JpNTEWb7CJWOZ7m2LX0GLu4Fsrwo=
github.com/google/uuid v1.6.0 h1:NIvaJDMOsjHA8n1jAhLSgzrAzy1Hgr+hNrb57e+94F0=
github.com/google/uuid v1.6.0/go.mod h1:TIyPZe4MgqvfeYDBFedMoGGpEw/LqOeaOT+nhxU+yHo=
github.com/jackc/pgx/v5 v5.11.0 h1:IzBBtyK9AHqf98cctWFifYSci2hgQR/cd56wB4p+ogg=
github.com/jackc/pgx/v5 v5.11.0/go.mod h1:mal1tBGAFfLHvZzaYh77YS/eC6IX9OWbRV1QIIM0Jn4=
go.opentelemetry.io/otel/metric v1.46.0 h1:yBnkXvgV7AXFILZc5K6IZe/CBFF3OS7BJ8ov6/lj0K8=
go.opentelemetry.io/otel/metric v1.46.0/go.mod h1:iPmdWqifKUdzziPkvvzIJXITl56fQx2mGM/DHLB3/2o=
go.opentelemetry.io/otel/trace v1.46.0 h1:OULy7ccdJnZtJ0UDYFOIGaCmiWzJ8Vi2G/Rsu60qs1c=
go.opentelemetry.io/otel/trace v1.46.0/go.mod h1:J7GAXweO77XSFkB/rmAqk9D6ihszhFjLU+d9WuUxDLI=
go.opentelemetry.io/otel/sdk/metric v1.46.0 h1:0piZ26EG4RBfebb2jhDH6ERCYHoVWduc3kLgPCwSnSE=
go.opentelemetry.io/otel/sdk/metric v1.46.0/go.mod h1:I1PbKrdVc8Qu8HYVDNtqVIwLwjNrhsV/uFuxfwg8mO4=
```

## npm packages

The table records exact direct versions and registry SHA-512 integrity values. Metadata source for each row is `https://registry.npmjs.org/<package>/<version>`; the SRI is `dist.integrity`. `npm ci` will verify the actual tarballs when installation is authorized. No npm package tarball was downloaded here.

| Package | Exact version | Registry integrity |
|---|---|---|
| `npm` | `11.19.0` | `sha512-SDd/hHg3KqHE5Ht2NHWxNYNtqCQ2pXAPLl6OtQhPyED5PHsRfrOtO199MZTIG2cQoQ1ZRI9t28shrD+2cr3AAw==` |
| `react` | `19.3.0` | `sha512-E8LUcbtBWt20bbl2YoHfx4ZDBdxVTfOKtCZn9cDSJ4l6/nuoApcpIBcj47t2wZoVX8g2ZHuMHbiShgCR1T5Sog==` |
| `react-dom` | `19.3.0` | `sha512-JDk8dgif51OjFoDE70+OT9ICyYr+69HlmihNwp1+Nsfbna3t5sIiCa9ZJktDmQ4/1b/rn26hIAR2uYXDMr5r0Q==` |
| `react-router-dom` | `7.18.3` | `sha512-ytVbyBBM7vMfRCam25r0WMhSVSom909A8p+8m0/f1w853dz/xfFu6etAT2SEbVoSnI+ZoPRDqIsQXVT89gp7kg==` |
| `@tanstack/react-query` | `5.102.8` | `sha512-TYBea4OuXWD7MhaSHq069TWbFe7rcwWN6kzT7JF0OKi1K6c1gTv2IzD6A6ExJsCMozdkqBWeuIUZmu4KQg0O5A==` |
| `react-hook-form` | `7.88.0` | `sha512-QRaLOWhX93YCnMiRfnOFRSwWXZNt8qhm2JTZwypoDvKpSffTJHmpzMXt8U6PV5UThvL3IiiLDUWe2nHMtz6Mmw==` |
| `@hookform/resolvers` | `5.9.1` | `sha512-7b7vsbraJxKgjVSA1Nur9tLwj539WGJUBLA7QNvXnFoT2pM5Z7G+6rlukk4B2/QrTZy6huRtH6wKeESPKuIr6w==` |
| `zod` | `4.6.5` | `sha512-v5l/aFXZQeai4awLbOpSoHecE9UiMrnfx75tEXLjNonXVARxQ5mOeipTjROUchszUNCqnE+hqAMujRsRHsut2Q==` |
| `@radix-ui/react-dialog` | `1.1.23` | `sha512-Ksw4WeROkO4rC9k/onilX/Ao2Cr1ku1unMNH+XSCcP4jSXYu7HDsg9n4ojMjVb22XpYjAQ9qfrFlVbru1vXDUA==` |
| `typescript` | `6.0.3` | `sha512-y2TvuxSZPDyQakkFRPZHKFm+KKVqIisdg9/CZwm9ftvKXLP8NRWj38/ODjNbr43SsoXqNuAisEf1GdCxqWcdBw==` |
| `vite` | `8.3.0` | `sha512-lhZBVvEHefgE+HQZC9O7EBJgCU/nVzFNl7vkS4RE0APtWLP02/8QVIkQtzBxPquh7lq5/78NHipTj7ODQ6XuyQ==` |
| `@vitejs/plugin-react` | `6.1.1` | `sha512-yxLaQV9gkhS8ezJqCM6+ndU7mDY6gqAg75NQ+0IjwEI8IYOmQCgkRwHKVSfWXW076DsqMo0Dk+0FK1U+M5RgFw==` |
| `vitest` | `4.1.11` | `sha512-fhACrNXUidIbGSBr5FlbuBkO7VWC1ZyLl0DO4CU2DrQoAPxX84Ysxs+HeGQpii5lZWV1Q4gBZTTu49mF+A6Edw==` |
| `@vitest/coverage-v8` | `4.1.11` | `sha512-8MVGEFnJIcdGjcbfKmeq8z0pZHH0JlVtoVZH9Q/qwUp6wyFnEJUBMrw9DCaj+ra3vShGmhavjalMIhPNxZAUcw==` |
| `eslint` | `10.10.0` | `sha512-NPXn6r5zl4uET1DAVPaOwzX3rut4c0wcmw3dWJAfOsTM5+TogXo0DDjz8pwm/hL8cyVNpHqeK4JpN0NjnyFFNw==` |
| `@eslint/js` | `10.0.1` | `sha512-zeR9k5pd4gxjZ0abRoIaxdc7I3nDktoXZk2qOv9gCNWx3mVwEn32VRhyLaRsDiJjTs0xq/T8mfPtyuXu7GWBcA==` |
| `typescript-eslint` | `8.70.0` | `sha512-P/W5cz70/cQAuKfY3xwQMWWTV7BvJ0mAQmi+9mBcsVPaBUpd6Ohpa+fECv9rBFrQcig86jAiNBFNWUqnTjr4pw==` |
| `eslint-plugin-react-hooks` | `7.1.1` | `sha512-f2I7Gw6JbvCexzIInuSbZpfdQ44D7iqdWX01FKLvrPgqxoE7oMj8clOfto8U6vYiz4yd5oKu39rRSVOe1zRu0g==` |
| `eslint-plugin-react-refresh` | `0.5.7` | `sha512-XhJSzLljuYD4UjNuFGJu2v7aD3sqNVK12w5/DCUfrNfHWegZo2+TsavNx8MuxMwEquuFAzNbNfXKU0WoZ+YUIg==` |
| `globals` | `17.12.0` | `sha512-cezEd/DTyyht9cvSSURyygXPfy04GtWO/5e6ZPvH7fCtjKz9PYOmuawphw1Ctd1f6C+5JypXfGD7ahNMXvevBA==` |
| `@types/node` | `24.13.4` | `sha512-YJ7EqCstVTzIr0fMr7qul/977en+pQHrfmuKIo6Zr9i75Be21dr3MovcfvGtyvi2HAUrRerWps5sMO9I7WaxDw==` |
| `@types/react` | `19.3.0` | `sha512-N0rFCuH9YoxG9/m61l9MfpJKfmLOVU0em7ipIz6TRgSSkvReLB9vL85GB+yr8Bs5leqpvg96JSwF4ZS1s4viQg==` |
| `@types/react-dom` | `19.3.0` | `sha512-ZI7bU42mZXXKHn/qNLEw2IrbiINU7X5+vfgdixBHkCNpYWXjKgfQ/P+uyGb5CjOLB9UcnTeg3rylQtV2hym44Q==` |
| `jsdom` | `30.0.1` | `sha512-52v7mUVUfNQVYYqE1lcdaymWL0njO7lTLUog6ZvW2U5KsbiLk/GnZlVJ+qx0xfNJZ6Gn+KSpPNE52vurbxZwrA==` |
| `@testing-library/react` | `16.3.3` | `sha512-Uo193NgQbPMz6lrrhtRQQFcMC6Re/ELLFbbuVL30WDlZxlpZf9/lMHTAVxPRLw1q1iu9OJmR1c2BLiENRstdBg==` |
| `@testing-library/dom` | `10.4.2` | `sha512-yzr2S9HyAIdhz2/6qHgbs665Q7PKVcDF05vsOlHPxG1mo36gKVesdYVeDLnXgfjJ03CrKRk08knc6+E/9m8v2Q==` |
| `@testing-library/user-event` | `14.6.7` | `sha512-MPCpX8bxe8zS+JmmTwLp8jd0dy1rAm60Te/SL8JrQM3qvQJcBOs1d7IefJMyZzqM3EWBrDn/LWDt1BCGu4ASfg==` |
| `@testing-library/jest-dom` | `7.0.1` | `sha512-oMDTC3oA+6CXSO2JZnvOI7CA6oVub6kij5ggk9ohwye5slmkwxYDXcPOVxgMw/RQlticjtO0C1RZkR97HgrWMw==` |
| `@playwright/test` | `1.63.0` | `sha512-oxMK4vllB9RK5NQ2l1pq1IfOf2AvnEuj/vYGDj0H2nMtmtZpKtCwt/l00GEO6xjGfpBNAvjovvYdCm50dRQkpQ==` |
| `msw` | `2.15.0` | `sha512-2wQAmKkQKxRuXvYJxVhPGG0wZNBQyD06oJvxqw90XqLvptdqxdlHrFUfEteKkpaNORX3Xzc+HtEl/q0nfmN2wQ==` |

MSW pin added 2026-09-15 for the already approved §9 schema-validated fixture
architecture and T-035. Node `>=18` and TypeScript `>=4.8.x` requirements fit
the existing compiler pins. See the [ADR-13 amendment approval](../state/approvals/T-035-msw-pin.md).

Preserve existing documentation-tool pins `clean-css` 5.3.3, `html-minifier-terser` 7.2.0 and `terser` 5.44.0, and their existing package-lock integrity entries. They are existing observations, not new production dependencies. Preserve all root `mocks:*` and `doctor` commands. Keep future workspace libraries/build tools in devDependencies unless needed by the browser app at runtime. npm itself is the package manager pin, not a new application dependency.

Compatibility decisions checked against exact registry engines/peerDependencies:

- Node 24.21.0 satisfies Vite/plugin-react, Vitest 4.1.11, ESLint 10, jsdom 30 and all listed tooling engine ranges. @types/node is pinned to the Node 24 line, not the latest Node 26 declarations.
- TypeScript **6.0.3** satisfies typescript-eslint 8.70.0's `>=4.8.4 <6.1.0`. Registry latest TypeScript 7.0.2 does not, so it is excluded.
- ESLint 10.10.0 satisfies @eslint/js 10.0.1, typescript-eslint, React Hooks and React Refresh plugin peers; use flat configuration.
- Vite 8.3.0 satisfies plugin-react 6.1.1 (`^8.0.0`) and Vitest 4.1.11 (`^6.0.0 || ^7.0.0 || ^8.0.0`). Coverage uses the matching 4.1.11. Optional React compiler/Babel, Sass, canvas and Vitest browser adapters are not required by this baseline.
- React and react-dom are both 19.3.0; matching type packages satisfy their peer ranges. Router, Query, RHF, Radix and Testing Library accept React 19. RHF 7.88.0 and Zod 4.6.5 satisfy resolver 5.9.1's required peers. Non-Zod resolver integrations are optional.
- Testing Library React 16.3.3 pairs with DOM 10.4.2; jest-dom 7.0.1 accepts this DOM range and Vitest. Playwright 1.63.0 is for later browser QA only; browser installs belong to that later setup task.

T-004 retains ownership of `web/vitest.workspace.ts` despite the historical name. Treat it as an **explicit config file** using `defineConfig` from `vitest/config` and `test.projects`; invoke `vitest run --config web/vitest.workspace.ts`. Do not use the retired `defineWorkspace`/workspace auto-discovery API. [Vitest v4 configuration](https://v4.vitest.dev/config/) and [projects](https://v4.vitest.dev/guide/projects) document this configuration shape. Keep paths rooted deliberately so app configs under `web/apps/*` and shared packages are discovered as they appear. An empty scaffold may report no tests; that is not a passing application test suite. Keep per-workspace build/typecheck and root orchestration separate from reference mock tests.

## Containers

Use full `docker.io/<repository>:<tag>@<index-digest>` references. Below, each SHA-256 was recomputed from the raw registry response and matched `Docker-Content-Digest`. Child manifests for Linux AMD64 and ARM64 were read from that exact index; unknown/unknown attestation entries are not runnable platforms. No image layers were pulled and no daemon state was changed.

| Purpose / repository:tag | Index digest | linux/amd64 manifest | linux/arm64 manifest |
|---|---|---|---|
| `docker.io/library/golang:1.27.1-alpine3.24` | `sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125` | `sha256:f86f1a6701e3dcc445fec097a42f78b758f15950ccf032c2d3e54e2754d32fdb` | `sha256:df4c4a0eeb85873e0122c6e2eb1b436f3131576f572505c1ea61954b00fa6460` |
| `docker.io/library/alpine:3.24.1` | `sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b` | `sha256:79ff19e9084a00eece421b2523fb93e22d730e2c0e525905de047e848e56d95f` | `sha256:e7a1a92a5bfeee40966aea60f0796b0e7917cc35591542701834f03a68fa3d18` |
| `docker.io/library/postgres:18.6-alpine3.24` | `sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2` | `sha256:63bdc97d67b5133bf0e5ebd500bec6d046fa851dc81340d838f0347e616107e8` | `sha256:cbe15165195f7f2d63885b4d990fdec7b602248533cb05bd992284a45a58fed3` |
| `docker.io/library/rabbitmq:4.3.5-management-alpine` | `sha256:b3b8b7f95f5382a19f9ea33540e604f30aad081d37ad9aba72255135765373a1` | `sha256:109225ae255547cf13a89c6b7d72f8f78826e4f3fd00a1c8dfbad5bacad70a68` | `sha256:efdae0bb03e4c9ab5e24e0672c7988a6a5c3d4db2c1f74ff225652a636b501bf` |
| `docker.io/clamav/clamav:1.4.6_base` | `sha256:2aed1ac770d2ef4ef2e1e25cab65fc04582244459328255b122fc9de5b3fb307` | `sha256:c05bf5d5a291f75d5570239bbab523c74f0621461d80a4aeeab1581a27814276` | `absent` |
| `docker.io/library/node:24.21.0-alpine3.24` | `sha256:be80f76cf40ec8e42b9bec49f60a55e0660f30af58d3e5a25530785b30ea67e2` | `sha256:333f6b3eca25980d5682c26207665b93c9417786b21760b2764d5821d9704c8a` | `sha256:c90fbae51ca047f2fda9ea92fb85eb936c08e6df18df7462bb782bad7c6afa3d` |

Registry evidence URL format: `https://registry-1.docker.io/v2/<repository>/manifests/<tag-or-digest>`, with a temporary anonymous pull token from `https://auth.docker.io/token?service=registry.docker.io&scope=repository:<repository>:pull`. Tokens were not stored in this proposal. Request OCI index and Docker manifest-list media types, hash the raw response body before JSON parsing, and compare its header. Tag mutability is why the digest is mandatory.

- Go builder and Alpine runtime: build separate non-root owner/edge images. Package checksums do not establish runtime non-root behavior or CA/timezone availability; Dockerfile tasks must verify those. Prefer pure Go builds for the selected driver stack; race-test builds require the host C compiler.
- PostgreSQL 18 image metadata declares `PGDATA=/var/lib/postgresql/18/docker`. Follow the image's versioned volume layout in T-005; do not reuse pre-18 mount assumptions. Preserve seven databases/users and owner-only grants.
- RabbitMQ includes management for local loopback diagnostics only. Its digest fixes embedded Erlang/OpenSSL bytes; no independently floating Erlang package installation. [Compatibility guidance](https://www.rabbitmq.com/docs/which-erlang) permits OTP 27, and OTP 28 only for brand-new clusters; mixed upgrades are not covered. Validate embedded OTP/version at T-006 startup before readiness. Keep mandatory routing, persistent delivery and confirms; server version is no substitute for outbox/inbox tests.
- ClamAV `1.4.6_base` has **only linux/amd64** in its index. On this Apple Silicon laptop the scanner Compose service must declare `platform: linux/amd64`; record emulation during scanner QA. No native ARM64 or scanner throughput claim is made. The `_base` image omits signature databases: initialize/update through FreshClam with a private signature volume, and fail readiness/scan acceptance until signatures are usable. Never turn an unavailable scanner into a clean result. Engine digest is immutable; signature identity/version/time remains separately recorded evidence, not frozen forever. See [vendor Docker instructions](https://docs.clamav.net/manual/Installing/Docker.html).
- Node image is a frontend build tool, not a production business service. Local development can use the verified native Node archive.

Architecture's telemetry collector/Jaeger profile is optional and remains disabled in the baseline; no `latest` collector is introduced. Core services can export OTLP to a configured endpoint later. Pin any optional collector in its own bounded task before enabling that profile. Private filesystem storage needs no MinIO/S3 emulator; no production S3 vendor, Redis, gRPC, Kubernetes or cloud platform is introduced. Migrate is built from the pinned Go module, not an additional migration container.

## Security evidence and later checks

The official npm bulk-advisory endpoint `https://registry.npmjs.org/-/npm/v1/security/advisories/bulk` returned `{}` for the proposed direct npm versions on 2026-09-14. This request contained only public package names/versions. It did **not** inspect existing mock-tool transitives or the future resolved dependency graph, and is not a claim of zero vulnerabilities.

Go release notes, Node's OpenSSL/Undici updates, PostgreSQL release notes and ClamAV vendor security fixes support choosing the current supported patches above. Container OS packages and application reachability were not scanned. Before merging executable consumers, run the pinned `govulncheck`, `go mod verify`, npm audit against the actual lock and relevant build/tests; report findings and reachable impact without silently suppressing them. A dependency change remains serialized and reviewed under ADR-13. Recheck supported series before upgrades and before release; RabbitMQ's 2026-11-30 community deadline is the earliest listed engine support deadline.

## Argon2 benchmark plan — not production parameters

Confirmed ADR-09 uses Argon2id with versioned parameters. [Go Argon2 documentation](https://pkg.go.dev/golang.org/x/crypto/argon2) and [RFC 9106](https://www.rfc-editor.org/rfc/rfc9106.html) provide algorithm/API guidance; they do not define this product's latency, resource or recovery policy.

1. Use `golang.org/x/crypto/argon2.IDKey` at the selected version, crypto/rand salts, constant-time verification and an encoded record containing algorithm/version, memory, iterations, lanes, salt and output. Use synthetic passwords only; benchmark output contains no credentials.
2. Begin measurements with RFC 9106's memory-constrained Argon2id profile: memory 64 MiB, iterations 3, lanes 4, 16-byte salt and 32-byte output. This is a benchmark candidate, not an accepted production setting. Include a measured sweep of memory/iterations/lane combinations permitted by the later security review; never lower approved parameters automatically under load.
3. Measure hash and verify p50/p95/p99, RSS/allocations and CPU under serial and bounded concurrent load on darwin/arm64 plus actual linux/amd64 and linux/arm64 service resource limits. Record compiler, package version, CPU, cgroup limits, GOMAXPROCS and concurrency. Emulated scanner measurements are unrelated to identity hashing.
4. Test rejection/bounds for malformed encoded hashes, extreme memory/iteration requests, constant-time comparisons, bounded login concurrency and rehash-on-success when a reviewed parameter version changes. Include wrong-password and non-existent-account work paths without enumerating real users.
5. Submit evidence to T-002/security acceptance for the resource/latency budget and final parameter choice. No measured benchmark or final session/recovery setting is claimed here; this plan does not block unrelated scaffolding.

## Remaining boundaries and approval

Ownership, delivery and transactions remain the approved seven-owner / HTTP+RabbitMQ / local atomic transaction + durable outbox/inbox contracts. No new ownership or reliability decision is needed for these pins. Financial calculation/rounding rules, retention, live provider/security acceptance and other open decisions remain scoped to their existing tasks; this lock invents no business policy.

T-001 completed independent QA and coordinator approval at commit `2908e9ee82fdce240d0161ab33cae858ea2007ca`; see its approval and QA records. The T-004 Vitest correction must be reviewed as part of the final T-004 commit before integration. Real scanner behavior, service runtime compatibility and Argon2 measurements remain downstream evidence. Neither this lock nor the tooling correction completes B-01.AC1, an application or a production deployment.

## T-004 correction — 2026-09-14

Actual strict TypeScript compilation exposed missing/broken published declarations
in Vitest 5.0.0 that the T-001 engine/peer metadata review did not detect.
The matched 4.1.11 pair passed the developer's strict configuration and test-source
compilation, workspace execution, coverage and full dependency audit. Independent
metadata/SRI/advisory checks support this correction; exact-commit T-004 QA remains
required. No `skipLibCheck`, dependency patch or extra declaration shim is used.
Vitest v4 is a prior major with recent maintenance evidence, not an LTS promise.
Keep project inheritance and mock-reset defaults explicit; do not rely on v5
defaults. See the correction approval for provenance and version-specific sources.
