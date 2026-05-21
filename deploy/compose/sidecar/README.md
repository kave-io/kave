# Kave Sidecar

Drop-in Kave service for deployments that already have Postgres. This compose
file only starts `kave-server`; point it at the existing database through
`.env`.

```bash
cp .env.example .env
$EDITOR .env
docker compose up -d
docker compose logs -f kave-server
```

Use this variant when Kave should run alongside an existing app stack, such as
Simorq production compose.
