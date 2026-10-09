# Releasing

## Continuous GHCR images

`.github/workflows/ci.yml` publishes an image after successful secret scans,
tests and the container smoke check on `main`. A manual CI run on `main` can
also rebuild it. Pull requests do not log into GHCR or publish images.

- Registry: `ghcr.io/syt100/notify-relay`.
- Tags: `main` follows the latest passing main commit; `sha-<full-commit-sha>`
  identifies the source commit. Images can be rebuilt, so use the manifest
  digest from the job summary when you need an immutable deployment reference.
- Platforms: `linux/amd64`, `linux/arm64`, `linux/arm/v7`.
- Metadata: OCI source/revision labels, SBOM and provenance attestations.
- Authentication: the job's automatic `GITHUB_TOKEN` with `packages: write`;
  no personal access token or notification credentials are required.

New GHCR packages default to private, even when the repository is public.
After the first successful build, open
[the package page](https://github.com/users/Syt100/packages/container/package/notify-relay),
choose **Package settings**, then **Change visibility** → **Public** if you
want anonymous pulls. If the package already existed outside this workflow,
grant the repository write access under **Manage Actions access**.

Deploy with `compose.ghcr.yaml`; configure `.env` and the existing Docker
network as described in the README. Set `NOTIFY_RELAY_IMAGE` to a SHA tag,
version or digest to pin it. The default `main` tag is a development build.
The `latest` and semantic-version tags remain owned by the release workflow.

## Versioned releases

Before the first versioned release:

1. Confirm the target commit and release version.
2. Enable Actions and GHCR package publishing. Make the GHCR package public if
   anonymous image pulls are intended; repository visibility alone is not enough.
3. Enable private vulnerability reporting and branch protection on `main`;
   require the CI checks. Independent maintainers may keep required approvals
   at zero. Enable Dependabot/security updates as available.
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
workflow are derived from the repository/account that runs it.
