# Kave core

This module contains the pure-Go kernel domain contract: tenant scopes,
manifests, capabilities, admission, limits, provider activation, accounting,
reporting, service-key material, and validation.

It has no network server or persistence implementation. Those boundaries live
in `server/internal/v2`; canonical wire types live in
`proto/kave/kernel/v2`.

```sh
go test -race ./v2/...
```
