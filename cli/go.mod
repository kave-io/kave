module github.com/kave-io/kave/cli

go 1.26.1

require (
	github.com/kave-io/kave/core v0.0.0
	github.com/kave-io/kave/proto/gen v0.0.0-00010101000000-000000000000
	github.com/spf13/cobra v1.10.2
	go.yaml.in/yaml/v3 v3.0.4
	google.golang.org/grpc v1.80.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/cpuguy83/go-md2man/v2 v2.0.6 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/zalando/go-keyring v0.2.8 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

replace github.com/kave-io/kave/core => ../core

replace github.com/kave-io/kave/proto/gen => ../proto/gen
