module github.com/kave-io/kave/connectors

go 1.26.1

require (
	github.com/google/uuid v1.6.0
	github.com/kave-io/kave/core v0.0.0
	github.com/tidwall/gjson v1.18.0
)

replace github.com/kave-io/kave/core => ../core
