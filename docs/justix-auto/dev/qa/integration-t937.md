# T-937 coordinator integration

2026-09-16. Exact independently GREEN candidate
`1636aeb291007163f80e41855694a2390a7694f5` was merged as
`9708a2330f45547ee09d37cc67451524321775a2`.
The three reviewed source/test/result leaves and all twenty artifacts across
original BOUNCE and correction GREEN are byte-identical to their reviewed sources.
See [independent correction review](T-937-r2.md).

On merged main, Go 1.27.1 with approved Node 24.21.0 and offline caches:
`bash tools/go.sh test -race -mod=readonly -count=1 ./tests/contracts`
passed in 117.255s; `bash tools/go.sh vet -mod=readonly ./tests/contracts`
exited 0. Logs are `/private/tmp/justixauto-t937-main-race.log` and
`/private/tmp/justixauto-t937-main-vet.log`. SHA-256 respectively:

```text
c85860cc6a063e13db1431ad343d4ef1a0dbe891869f22a81b6e6382f33faa17
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

This integrates the closed, inert auth profile parser. Public generation and
both renderers still reject auth through T-941. T-942 activation, concrete auth
bindings, session integration and release-policy acceptance remain separate.
