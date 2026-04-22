module github.com/kave-io/kave/app

go 1.26.1

require (
	github.com/kave-io/kave/core v0.0.0
	github.com/kave-io/kave/proto/gen v0.0.0
	go.yaml.in/yaml/v3 v3.0.4
	google.golang.org/grpc v1.75.1
	google.golang.org/protobuf v1.36.10
)

require (
	github.com/kr/pretty v0.3.1 // indirect
	github.com/oklog/ulid/v2 v2.1.1 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250825161204-c5933d9347a5 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

replace (
	github.com/kave-io/kave/core => ../core
	github.com/kave-io/kave/proto/gen => ../proto/gen
)
