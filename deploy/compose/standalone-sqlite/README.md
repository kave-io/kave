# Kave Standalone SQLite

Fastest self-hosted Kave setup. App state uses SQLite and span data uses
DuckDB on a named Docker volume.

```bash
cp .env.example .env
docker compose up -d
docker compose logs -f kave-server
```

Open `http://127.0.0.1:18080` for the dashboard. The gRPC API listens on
`127.0.0.1:19090`, and the SDK proxy listens on `127.0.0.1:18081`.

Pin production installs by setting `KAVE_VERSION=0.2.0` in `.env`.
