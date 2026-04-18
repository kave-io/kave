module github.com/kave-io/kave/connectors

go 1.26.1

require (
	github.com/google/uuid v1.6.0
	github.com/kave-io/kave/core v0.0.0
	github.com/tidwall/gjson v1.18.0
)

require (
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
)

replace github.com/kave-io/kave/core => ../core
