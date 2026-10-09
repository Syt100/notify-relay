# Releasing

The project is prepared for GitHub, but no repository, registry namespace or
public release is assumed to exist yet. Before the first release:

1. Create/import the repository and choose its owner/name.
2. Enable Actions and GHCR package publishing. Make the GHCR package public if
   anonymous image pulls are intended; repository visibility alone is not enough.
3. Enable private vulnerability reporting and branch protection on `main`;
   require CI checks and review. Enable Dependabot/security updates as available.
4. Optional Docker Hub publishing: create `notify-relay` under your Docker
   Hub account and add `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` Actions secrets.
5. Run `make check`, complete the container smoke check and verify one real ntfy
   to WxPusher message, including a reconnect and container restart.
6. Move the relevant changelog entries to a version heading and push a semantic
   version tag, for example `v0.1.0`.

Tag pushes run `.github/workflows/release.yml`: verification, three Linux binary
archives with license notices, SHA-256 checksums, multi-platform images with
SBOM/provenance metadata, and a GitHub Release. Only stable tags should be used
initially; prerelease-specific publishing policy is not configured.

Use the exact version or digest for deployments. For Portainer, replace
`build:` with the published GHCR or Docker Hub `image:` and keep environment,
network and volume configuration. No real notification credentials belong in CI.

GitHub cannot atomically publish images and releases together. If publishing
fails after an image is pushed, inspect the workflow output before retrying;
the binary artifacts and image might already exist. Do not delete or move an
already published release tag. Correct it with a subsequent patch release.

The local source package does not publish anything. Registry addresses in the
workflow are derived from the eventual repository/account rather than a
placeholder owner that users might accidentally copy.
