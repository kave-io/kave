# Kave console

The console is the small, read-focused operational UI embedded in
`kave-server`. It covers observability, usage analytics, opaque tenant
references, namespace state, and audit evidence. It contains no provisioning
or secret-management controls and never fabricates data.

```sh
bun install --frozen-lockfile
bun run type-check
bun run test
bun run lint
bun run format:check
bun run build
```

The focused browser suite uses Chromium:

```sh
bunx playwright install chromium
bun run test:e2e
```

The development server proxies the generated Connect API to
`http://127.0.0.1:8080`. Production serves the compiled assets and API from the
same origin. A service key remains in memory unless the operator explicitly
chooses tab-scoped session storage; local storage is not used.
