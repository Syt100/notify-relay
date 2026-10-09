# Contributing

Discuss significant changes in an issue before implementing them. Keep small
bug fixes focused and explain the trigger, expected result and reproduction.

1. Use Go 1.26 or newer and create a topic branch.
2. Add a regression test for changed behavior. HTTP tests use local test servers
   rather than sending real notifications.
3. Run `make check` and, for container changes, build and smoke-test the image.
4. Update configuration documentation and `CHANGELOG.md` for user-visible changes.
5. Open a pull request with the behavior change, validation and known limitations.

Do not commit tokens, `.env`, runtime databases, notification bodies or private
hostnames. Do not introduce a new dependency without explaining its purpose
and license. Preserve the small-server target, bounded memory use, atomic
checkpoints, retry semantics and credential-safe logs.

CI runs Gitleaks against the full Git history and checked-out files. Use obvious
dummy values in tests; do not suppress findings by broadly excluding source paths.

Use conventional commit prefixes such as `fix:`, `feat:` and `docs:` when useful.
Maintainers use squash merges. No CLA is required; contributions are provided
under this project's MIT license. Treat other contributors respectfully and
focus review on the code and observable behavior.
