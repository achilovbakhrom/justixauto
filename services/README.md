# Go services — reserved layout

No implementation exists yet. After architecture approval use the Gaze shape:
`<service>/cmd/<binary>/main.go`, `internal/domain`, `internal/app`,
`internal/port`, `internal/adapter`, `db/migrations` and public `pkg` as needed.
Separate deployment units do not require one Git repository per service.
Do not create a service per sidebar item. Approve ownership and APIs/events first.
