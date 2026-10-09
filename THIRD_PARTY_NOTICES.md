# Third-party notices

Runtime dependencies are pinned in `go.mod` and verified by `go.sum`:

| Component | License | Included notice |
| --- | --- | --- |
| Go standard library/runtime | BSD-3-Clause | `third_party/go-LICENSE` |
| go.etcd.io/bbolt | MIT | `third_party/bbolt-LICENSE` |
| golang.org/x/sys | BSD-3-Clause | `third_party/x-sys-LICENSE` |

These notices accompany distributed binary archives. Transitive test-only
modules are listed in `go.sum` but are not linked into the runtime binary.
ntfy and WxPusher are independently operated projects/services; this bridge is
not affiliated with or endorsed by either project.
