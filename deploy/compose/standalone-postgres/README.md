# Kave Standalone Postgres

Single-host setup with Postgres for Kave app state and DuckDB for local span
storage.

```bash
cp .env.example .env
$EDITOR .env
docker compose up -d
docker compose logs -f kave-server
```

Open `http://127.0.0.1:18080` for the dashboard. Pin production installs by
setting `KAVE_VERSION=0.2.0` in `.env`.
