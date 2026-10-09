# Architecture

This project forwards authenticated ntfy JSON streams to the WxPusher SPT API.
It targets small Linux servers and runs as a single Go binary without CGO.

## Delivery contract

- One ntfy stream per topic, with independent persistent checkpoints.
- Message insertion and checkpoint advancement share one bbolt transaction.
- Replay uses the saved Unix timestamp minus one second. IDs are deduplicated
  per topic, including pending messages and seven days of accepted records.
- On first startup, the ntfy stream's `open` time seeds the checkpoint; existing
  history is not replayed. Reconnect before the first message still recovers
  messages after that first connection.
- Pending payloads stay on disk. The worker fetches one due message at a time
  through an ordered index; it never loads the queue into memory.
- Failed requests retry indefinitely with bounded exponential backoff and
  jitter. All sends are globally paced to at most one request per second.
- Queue capacity defaults to 10,000 pending records. A full queue stops
  subscription consumption without advancing the checkpoint. Recovery relies
  on ntfy still retaining those unconsumed messages.
- Deduplication is local, not end-to-end exactly-once delivery. An accepted
- If a server reports a capped replay, processing continues, an error is logged,
  and topic readiness records a history gap until the process restarts. The
  missing historical records cannot be recovered by this bridge automatically.
- Deduplication is local, not end-to-end exactly-once delivery. An accepted
  request with a lost response or a crash before the local acknowledgment may
  produce duplicate notifications. API acceptance does not prove phone display.
- Attachment links and view actions become text; HTTP/broadcast actions are
  never executed. `click` maps to the provider URL.

## Operations

`/healthz` checks process liveness; `/readyz` includes all subscriptions and the
most recent provider outcome. Endpoints include queue counts but no secrets or
message bodies. One process owns one state file. Graceful termination cancels
streams and in-flight sends before closing the database.

Configuration comes from validated environment variables. Tokens support
`*_FILE` alternatives. Network redirects are disabled to avoid forwarding
credentials to another origin. The runtime image uses a non-root user, a
read-only root filesystem, and a writable data volume.

The state format is bbolt. Use a dedicated `STATE_DB` path; SQLite and unrelated
application databases cannot be used as state files.
