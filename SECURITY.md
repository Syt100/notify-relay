# Security policy

Only the latest tagged version is intended to receive fixes. Before the first
release, the main development branch is the supported target.

Never post credentials, notification content or runtime databases in a public
issue. If this project is hosted on GitHub with private vulnerability reporting
enabled, use the repository's Security tab to report a vulnerability privately.
If that option is unavailable, ask the maintainer for a private contact without
publishing exploit details. No response-time SLA is promised.

The ntfy token should be read-only. SPT grants the ability to send notifications;
protect and rotate it like a password. State files contain notification bodies
and link URLs, unencrypted. Use a protected local volume and secure backups.

Only `view` links are rendered; this service never executes action commands.
ntfy and provider servers are trusted configuration endpoints. No redirects,
TLS verification bypass or credential-bearing URLs are supported. Health
endpoints are unauthenticated and intended only for an internal network.
