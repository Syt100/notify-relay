# Configuration

All numeric values are positive integers. Seconds are used unless noted.
Configuration errors name the variable without printing its value.

| Variable | Default | Meaning |
| --- | --- | --- |
| `NTFY_BASE_URL` | `http://ntfy` | ntfy HTTP(S) base URL, optional path prefix |
| `NTFY_TOPICS` | `notify` | Comma-separated unique topics; letters, digits, `_`, `-`, max 64 characters each |
| `NTFY_TOKEN` | required | Token with read access to every topic |
| `NTFY_TOKEN_FILE` | unset | Read token from a UTF-8 file instead |
| `WXPUSHER_SPT` | required | One WxPusher SPT |
| `WXPUSHER_SPT_FILE` | unset | Read SPT from a file instead |
| `WXPUSHER_API` | official `/api/send/message/simple-push` HTTPS endpoint | Alternate endpoint for tests or a trusted proxy |
| `STATE_DB` | `/data/bridge.bolt` | bbolt state file; incompatible with SQLite |
| `HEALTH_PORT` | `8080` | Bind health server on all container interfaces |
| `NTFY_STALE_SECONDS` | `180` | Cancel a stream after no valid events for this duration |
| `SEND_TIMEOUT` | `15` | Timeout per provider HTTP request; text fallback has its own request timeout |
| `RETRY_BASE_SECONDS` | `2` | Initial per-message retry delay |
| `RETRY_MAX_SECONDS` | `300` | Maximum exponential delay including jitter |
| `DELIVERED_RETENTION_DAYS` | `7` | Accepted/filtered ID retention in days |
| `FORWARD_MIN_PRIORITY` | `1` | Send only priority at least this value, between 1 and 5 |
| `MAX_PENDING` | `10000` | Maximum combined pending and isolated records before subscription backpressure |
| `LOG_LEVEL` | `INFO` | `DEBUG`, `INFO`, `WARN` or `ERROR` |
| `GOMEMLIMIT` | `32MiB` in image | Go runtime soft memory target; excludes mapped database pages |
| `GOGC` | `50` in image | Go GC tuning; lower values trade CPU for a smaller heap |
| `GOMAXPROCS` | `1` in image | Go scheduler CPU parallelism |

For either token, set the direct value or its `_FILE` variant, never both.
Only trailing/leading whitespace in credential files is removed. A Compose
secret can be mounted under `/run/secrets/` and referenced through `_FILE`.
The `.env` name `NTFY_WXBRIDGE_TOKEN` is a Compose substitution variable, mapped
to the program's `NTFY_TOKEN` environment variable.

Provider `Retry-After` on a failed response is respected globally, capped at
one hour, and can exceed the configured exponential maximum. ntfy reconnect
backoff is separate: 1–60 seconds. Queue-full retries occur after one second.
An event must fit the 1 MiB scanner limit; provider responses are capped at
64 KiB. ntfy's normal small notifications are comfortably below these limits.

URLs reject embedded credentials, query strings and fragments. Redirects are
not followed. The standard `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY` variables
are supported; set `NO_PROXY=ntfy,localhost,127.0.0.1` if using an outbound proxy.
HTTPS uses standard CA verification. Do not expose the health port publicly.

Connection/TLS timeouts are 10 seconds; ntfy response headers time out at 15 seconds. Queue
polling uses a 250 ms wait. Structured log timestamps use UTC.

## Failed queue maintenance

HTTP 400/413/422 failures are isolated immediately. Business code 1001 means a
generic rejection: five consecutive rejected delivery attempts trigger isolation.
Markdown rejection first gets one text fallback within the same attempt. Other
errors (including HTTP 401/403, business code 1002, 429 and network failures)
keep retrying. Provider response text is never saved in diagnostic metadata.
Isolation retains the original message in the private state database and keeps
`/readyz` unhealthy. It does not discard or mark the message as delivered.

Stop the bridge before maintenance; bbolt allows only one process per state
file. The commands reuse the normal environment, network and mounted volume:

```sh
docker compose -f compose.ghcr.yaml stop wxpusher-bridge
docker compose -f compose.ghcr.yaml run --rm --no-deps wxpusher-bridge failed list
# Fix the credentials, endpoint or other rejection cause before retrying.
docker compose -f compose.ghcr.yaml run --rm --no-deps wxpusher-bridge failed retry notify MESSAGE_ID
docker compose -f compose.ghcr.yaml up -d wxpusher-bridge
```

`failed list` shows a total count and at most 100 records with topic, ID,
attempts, time and sanitized reason. It does not print message bodies. Requeue
listed records before listing the next batch. `failed retry TOPIC ID` preserves
content and resets attempts/rejection counts. For a local build use `compose.yaml`
instead. With a bind mount, keep the same UID/GID and directory permissions.
There is no automatic deletion of isolated messages.

Back up the state file while stopped before upgrading. Schema 1 upgrades to 2
automatically; older binaries refuse schema 2. Restore the pre-upgrade backup
when rolling back, accounting for notifications sent since that backup.

`NTFY_DOCKER_NETWORK` is a Compose-only variable selecting the existing Docker
network shared with ntfy (default `ntfy`); it is not a program setting.
