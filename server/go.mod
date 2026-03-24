module github.com/kave-io/kave/server

go 1.26.1

require (
  github.com/kave-io/kave/core v0.0.0
  github.com/kave-io/kave/connectors v0.0.0
)

replace (
  github.com/kave-io/kave/core => ../core
  github.com/kave-io/kave/connectors => ../connectors
)
