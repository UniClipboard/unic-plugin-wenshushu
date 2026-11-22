# unic-plugin-wenshushu

A UniClipboard Go Action plugin for temporary sharing through Wenshushu.

Invoke **通过文叔叔临时分享** on a history item. The plugin uploads the selected
content and returns the public download URL as a new text history item.
The source remains unchanged. Nothing uploads automatically.

| Input | Shared content |
| --- | --- |
| Text | UTF-8 `clipboard.txt` |
| Image | Original stored bytes, such as `clipboard.png` |
| Files | All selected files in one share, including empty files |

Directories are excluded. Text that contains a file path or URL stays text.
Duplicate or unsafe filenames fail before upload. There is no browser dependency
and no account configuration. Anonymous shares request a one-day expiry.
Live acceptance observed approximately 24 hours; provider quotas and policy apply.

## Install

Use a UniClipboard host that includes the Action source-reader and deadline
extensions from host commit
[`549ef96c8ab5dae7f5791d03a05a9726087de286`](https://github.com/UniClipboard/UniClipboard/commit/549ef96c8ab5dae7f5791d03a05a9726087de286).
These changes are on `feat/wenshushu-action-source`; they are not merged into
the host default branch. Older hosts cannot run this plugin.

```sh
unic plugin install https://github.com/UniClipboard/unic-plugin-wenshushu --ref feat/wenshushu-share --build
unic plugin enable unic.wenshushu
```

Restart the service after enabling the plugin, as required by the host installer.
Select a text, image or file history item, then invoke `unic.wenshushu/share`.
The CLI equivalent is:

```sh
unic h action @1 unic.wenshushu/share --apply
```

The optional `--apply` copies the result through the host's clipboard adapter.
The plugin never writes the clipboard directly. There are no prebuilt release
artifacts yet, so installation currently requires a compatible Go toolchain.

## Identity and limits

- Repository: `UniClipboard/unic-plugin-wenshushu`
- Plugin ID: `unic.wenshushu`
- Action key: `unic.wenshushu/share`
- Version: `0.1.0`
- Action deadline: five minutes
- Upload buffering: at most 2 MiB per sequential file chunk
- Selection bound: 500 files and 5 GiB total, subject to lower host/provider limits

The dotted plugin ID follows the host identity contract. Repository names use
`unic-plugin-<slug>`. Provider code lives here; generic SDK/RPC/Engine changes
live in `UniClipboard/UniClipboard`.

## Failure behavior

Provider errors, source failures, timeouts and cancellation do not produce a
success history item. Provider mutations are not retried automatically. A remote
share may survive a lost response or a later local history failure. Engine
RequestID deduplication does not promise exactly-once remote effects after a
service restart. Uploaded content and URLs are excluded from logs.

## Verify

```sh
GOWORK=off sh scripts/verify.sh
# Upload synthetic fixtures and download them as an independent recipient:
GOWORK=off WENSHUSHU_LIVE=1 sh scripts/verify.sh
```

Each run creates a unique artifact directory under `.cache/acceptance/`. It
contains source checksums, a source patch, test logs, JSON receipts, a binary
checksum and a replay command. Live URLs and tokens are redacted. Keep the first
failure and investigate it before retrying. CI runs deterministic provider and
real-plugin-process E2E; live network acceptance is opt-in.

See [ACCEPTANCE.md](ACCEPTANCE.md) for executed evidence and remaining limits.
