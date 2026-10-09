# Changelog

This project uses Semantic Versioning. Entries describe user-visible changes.

## Unreleased

### Fixed

- Preserve ntfy Markdown metadata and send Markdown to WxPusher, with a paced
  text fallback on rejection. Include topic source in summaries and bodies.
- Account for text line-break expansion and UTF-16 character limits; move
  oversized click URLs into the bounded body instead of sending invalid fields.
- Isolate HTTP 400/413/422 and repeated business rejections without dropping
  content. Add failed queue inspection/requeue commands and health counts.
- Correct the Chinese README's outdated image availability statement.
- Preserve BuildKit's target platform when cross-compiling container binaries.
  Registry publishing now checks each platform's actual ELF binary.

### Added

- Automatic multi-platform GHCR publishing after successful main-branch CI,
  with main/SHA tags, build metadata and a prebuilt-image Compose example.
- Initial Go bridge with transactional bbolt outbox and per-topic replay cursors.
- Persistent retries, ID deduplication, queue capacity and priority filtering.
- WxPusher SPT text mapping with tags, attachment/action links and click URLs.
- Credential file support, bounded network input and safe structured logs.
- Health/readiness endpoints, non-root container and Compose deployment example.
- Tests, CI, dependency updates and Linux multi-architecture release workflows.
