# Credential encryption at rest

Kave stores connector credentials encrypted on disk. The encryption boundary is the server-side master key managed by `core/pkg/keyring`.

## Key material

- The master key is 32 bytes.
- The preferred storage location is the OS keyring entry `kave/master-key`.
- If the OS keyring is unavailable and `KAVE_ALLOW_PLAINTEXT_KEY=1`, development fallback stores the master key in `~/.kave/master.key` with `0600` permissions.

## Threat model

- Protects against database dumps, disk snapshots, and accidental file disclosure.
- Does not protect a host compromise where the attacker can read the OS keyring or the process memory.
- Does not protect plaintext secrets while they are in use by the running process.

## What is not covered

- Short-lived secret material held in memory during request handling.
- Secrets written to logs by a buggy caller.
- Compromise of the developer workstation or server host itself.

## Operational assumption

Plan 14 treats encrypted credentials as safe at rest. That assumption only holds when the master key stays outside the database and the fallback plaintext key path is not used in production.
