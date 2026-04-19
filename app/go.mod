module github.com/kave-io/kave/app

go 1.26.1

require (
	github.com/kave-io/kave/core v0.0.0
	github.com/kave-io/kave/proto/gen v0.0.0
	google.golang.org/grpc v1.69.2
	google.golang.org/protobuf v1.36.1
)

require (
	github.com/oklog/ulid/v2 v2.1.1 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241104194629-dd2ea8efbc28 // indirect
)

replace (
	github.com/kave-io/kave/core => ../core
	github.com/kave-io/kave/proto/gen => ../proto/gen
)
