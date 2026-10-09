# Verification record

Verified on 2026-10-08 using Go 1.27.1 on Linux amd64. This is a local
verification record for the initial `dev` build, not a production reliability
claim. An independent third-party code review has not been performed.

## Automated checks

- Formatting, `go vet ./...`, race-enabled tests and `go mod verify` passed.
- 20 top-level tests exercise credential/config validation, UTF-8 and expanded
  text size, retry and retention, restart/dedup, capacity rollback, topic isolation,
  subscription checkpoints, quiet-stream cancellation, mapping, provider rejection,
  redirects, multi-topic filtering, health probes and CLI commands.
- Two final-review regressions reproduce and fix truncated-replay reporting and
  global `Retry-After` pacing, including when a message's own backoff is longer.
- Local statement coverage was 74.6%. This is an observation, not a quality gate;
  disk hardware failure and every startup/OS error path are not covered.
- Cross-compiled Linux amd64, arm64 and ARM EABI5 armv7 executables with
  `CGO_ENABLED=0`; `file` confirmed static linking. Only amd64 was executed.
- Compose and workflow YAML parsed successfully. Docker execution was unavailable
  in this environment; image builds/smoke checks are configured in CI and have
  not run on GitHub yet. Actual WxPusher, ntfy containers and phone delivery were
  not tested; HTTP integration tests use real local HTTP servers.

## Measured process memory

Settings: `GOMEMLIMIT=32MiB`, `GOGC=50`, `GOMAXPROCS=1`. Each scenario used a fresh
state file, one ntfy stream and loopback HTTP servers. Provider failure was
simulated with HTTP 503. Measurements read the bridge's Linux `/proc` RSS and
high-water mark after connection/queue readiness plus two seconds. The separate
test server's memory is excluded. The stripped amd64 binary was 7,930,016 bytes.

| Scenario | RSS | Peak RSS | State file |
| --- | ---: | ---: | ---: |
| Connected, no pending messages | 7,992 KiB (7.80 MiB) | 7,992 KiB | 32 KiB |
| 1,000 pending messages, 1,024-byte body each | 12,292 KiB (12.00 MiB) | 12,592 KiB (12.30 MiB) | 4 MiB |

These are short local measurements. HTTPS, more topics, larger messages, sustained
traffic and larger queues can increase usage. RSS is not the same as total cgroup
memory: container file cache and other charged pages also count toward Docker's
64 MiB sample hard limit. Go's memory setting is a soft runtime target and excludes
the database mmap. No comparative benchmark against other implementations was made.

Reproduce on Linux with Node.js installed on a development machine:

```sh
make build
node scripts/measure-memory.cjs
```

The harness writes `dist/memory-results.json`, uses dummy credentials and loopback
servers, checks graceful termination, and removes its temporary state files.
Node.js is used only for this optional development measurement script.

## Release checks still required

Before publishing a stable tag, run CI's container smoke test and test one real
deployment: normal delivery, ntfy disconnection/replay, bridge restart with a
pending message, priority filtering and phone notification/click behavior.
