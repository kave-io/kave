module github.com/kave-io/kave/connectors

go 1.26.1

require (
	github.com/kave-io/kave/core v0.0.0
	github.com/tidwall/gjson v1.18.0
)

require (
	github.com/oklog/ulid/v2 v2.1.1 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
)

replace github.com/kave-io/kave/core => ../core
