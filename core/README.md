# Kave Core

Pure Go contracts and mapping code shared by the server and CLI.

## Layout

- `runtime/` live execution types.
- `model/` persisted records.
- `pipeline/` interceptor chain.
- `connectors/` framework, model, tool, and protocol connector contracts.
- `mappers/` conversion boundaries between runtime and storage shapes.
- `pkg/` small shared primitives.

```bash
go test ./...
go build ./...
```
