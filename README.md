# NotifyRelay

[简体中文](README.zh-CN.md) · [Configuration](docs/configuration.md) · [Architecture](docs/architecture.md)

```sh
git clone https://github.com/Syt100/notify-relay.git
cd notify-relay
```

A small Go service that forwards self-hosted ntfy notifications to WxPusher's
SPT API. Keep ntfy as your notification entry point and isolate provider
credentials in this bridge. A single static binary, an embedded bbolt outbox,
and no external database server. Currently supports ntfy → WxPusher.

## Features

- Authenticated JSON subscriptions with automatic reconnect and cached replay.
- Transactional message persistence and checkpoints; topic-scoped ID deduplication.
- Disk-backed retries with exponential backoff, jitter and a global 1 request/s pace.
- Priority filtering, emoji summaries, tags, attachment/view links and click URLs.
- Liveness/readiness endpoints, structured logs and file-based secrets.
- Non-root container with a 64 MiB example limit; Linux amd64, arm64 and armv7 builds.

This is an initial implementation, not an established production-tested release.
See [verification](docs/verification.md) for measured results and limitations.

## Quick start with an existing ntfy container

The included Compose file connects to `http://ntfy` on an existing Docker
network. Set `NTFY_DOCKER_NETWORK` in `.env` to the network shared with ntfy
(defaults to `ntfy`), and adjust `NTFY_BASE_URL` if needed. It does not expose
a host port.

```sh
cp .env.example .env
chmod 600 .env
# Edit .env: set the Docker network, a read-only ntfy token and your WxPusher SPT.
docker compose up -d --build
docker compose logs -f wxpusher-bridge
docker compose exec wxpusher-bridge /bridge readycheck
```

Use a dedicated ntfy user that can **read** your subscribed topics, not an admin
token. ntfy's own users and ACLs remain managed by your existing stack.
Do not put real passwords/tokens in comments or committed files.

For a bind mount instead of the named volume:

```sh
mkdir -p data
sudo chown 1000:1001 data
chmod 700 data
```

Replace `bridge-data:/data` with `./data:/data` in `compose.yaml`.
The runtime image contains no shell or `wget`: use `/bridge healthcheck` and
`/bridge readycheck`. A plain binary is also available through the tag-based
release workflow once this project is published.

## Binary deployment

Build on a development machine, then copy the binary to your server:

```sh
make build VERSION=dev
export NTFY_BASE_URL=https://ntfy.example.com
export NTFY_TOPICS=notify
export NTFY_TOKEN_FILE=/path/to/ntfy-token
export WXPUSHER_SPT_FILE=/path/to/wxpusher-spt
export STATE_DB=./data/bridge.bolt
export GOMEMLIMIT=32MiB GOGC=50 GOMAXPROCS=1
./dist/notify-relay
```

Your server only needs the binary, credentials, a writable state directory and
system CA certificates for HTTPS. It does not need the Go compiler. The binary
logs UTC timestamps; notification content is not altered based on timezone.

## Message semantics

The ntfy `title` (or topic) becomes the summary; `message` remains plain text.
Priority 1/2/3/4/5 maps to 💤/ℹ️/no prefix/⚠️/🚨. This preserves the meaning in
the title; it does not change Android notification importance or bypass DND.
The body includes tags, attachment links and `view` action links. `click`
becomes WxPusher `url`. Attachments are not uploaded; HTTP/broadcast actions
are not executed. Unsupported event fields are ignored.

Messages below `FORWARD_MIN_PRIORITY` are processed without sending. The
health field `processed_retained` includes both accepted and filtered messages.

## Reliability boundaries

- First startup subscribes to new messages. Subsequent connections replay from
  the persistent timestamp with a one-second overlap and deduplicate by ID.
- Pending messages have no expiration or retry limit. Accepted/filtered IDs
  are kept seven days by default. Replay older than this window can duplicate.
- Recovery before local persistence depends on ntfy cache retention. With
  caching disabled, expired cache, a full queue or an extended outage, upstream
  messages can be lost. Monitor `/readyz` and `pending`.
- If ntfy caps a replay (`X-Messages-Truncated: 1`), the bridge continues with
  the returned messages, logs the missing-history gap, and keeps `/readyz`
  unhealthy with `replay_truncated: true` until restart. Investigate the gap
  before acknowledging it by restarting; older omitted messages are not recovered.
- Delivery is at least once while messages remain available locally/upstream.
  WxPusher has no client idempotency key here; ambiguous network outcomes can
  duplicate. An accepted API request is not proof of phone notification display.
- All HTTP and business failures retry. A wrong SPT or invalid message needs
  operator correction; it will not be silently discarded. Other due messages
  can continue while that message waits for its retry.
- Run only one process per state file; do not put bbolt on network filesystems.
  The file may retain its disk high-water mark after record cleanup.
- The state file uses bbolt, not SQLite. Do not point `STATE_DB` at another
  application's database.

## Development and releases

Go 1.26+ is required. The build/CI toolchain is pinned to 1.27.1 and Dependabot
tracks Go modules, GitHub Actions and Docker image updates.

```sh
make check
make build
docker build -t notify-relay:dev .
```

Pull requests scan Git history and files for secrets, then run formatting, vet, race tests, coverage and a container smoke
check. Pushing a `v*` tag runs verification, creates Linux archives/checksums
and a GitHub Release, and publishes amd64/arm64/armv7 images to GHCR. Docker Hub
is optional: configure `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` repository
secrets. Neither registry requires notification credentials for building.
See [releasing](docs/releasing.md) before your first tag.

[Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [Changelog](CHANGELOG.md) · [MIT license](LICENSE)

Protocol references: [ntfy subscription API](https://docs.ntfy.sh/subscribe/api/),
[WxPusher SPT](https://wxpusher.zjiecode.com/docs/spt.html),
[bbolt](https://github.com/etcd-io/bbolt).
