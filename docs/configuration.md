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
| `SEND_TIMEOUT` | `15` | Overall provider request timeout |
| `RETRY_BASE_SECONDS` | `2` | Initial per-message retry delay |
| `RETRY_MAX_SECONDS` | `300` | Maximum exponential delay including jitter |
| `DELIVERED_RETENTION_DAYS` | `7` | Accepted/filtered ID retention in days |
| `FORWARD_MIN_PRIORITY` | `1` | Send only priority at least this value, between 1 and 5 |
| `MAX_PENDING` | `10000` | Maximum pending records before subscription backpressure |
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

`NTFY_DOCKER_NETWORK` is a Compose-only variable selecting the existing Docker
network shared with ntfy (default `ntfy`); it is not a program setting.
