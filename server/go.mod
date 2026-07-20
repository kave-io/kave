module github.com/kave-io/kave/server

go 1.26.5

require (
	connectrpc.com/connect v1.19.1
	github.com/jackc/pgx/v5 v5.9.2
	github.com/kave-io/kave/core v0.0.0
	github.com/kave-io/kave/proto/gen v0.0.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/oklog/ulid/v2 v2.1.1 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)

replace (
	github.com/kave-io/kave/core => ../core
	github.com/kave-io/kave/proto/gen => ../proto/gen
)
