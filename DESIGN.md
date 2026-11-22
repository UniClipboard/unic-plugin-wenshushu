# Wenshushu temporary-share Action design

- Status: Implemented in the independent repository. Synthetic live provider round trips and real-plugin-process E2E passed. Host extensions are required; native UI and release packaging remain pending.
- Date: 2026-11-22
- Scope: One independently distributed Go Action plugin and the generic host capabilities it needs.
- Landing targets: [Action contract](https://github.com/UniClipboard/UniClipboard/blob/bddd5223af0ae5a30f986632b2cba900774ada9e/docs/contracts/clipboard-action-contract.md), [plugin contract](https://github.com/UniClipboard/UniClipboard/blob/bddd5223af0ae5a30f986632b2cba900774ada9e/docs/contracts/plugin-contract.md), [RPC contract](https://github.com/UniClipboard/UniClipboard/blob/bddd5223af0ae5a30f986632b2cba900774ada9e/docs/contracts/plugin-rpc-contract.md), [SDK extension contract](https://github.com/UniClipboard/UniClipboard/blob/bddd5223af0ae5a30f986632b2cba900774ada9e/docs/contracts/sdk-extension-contract.md) and [Engine contract](https://github.com/UniClipboard/UniClipboard/blob/bddd5223af0ae5a30f986632b2cba900774ada9e/docs/contracts/engine-contract.md).

## Product behavior

The user selects one history item and invokes “通过文叔叔临时分享”. The plugin uploads that item's content and returns the provider's public download URL. The Engine commits the URL as a new text history item with Action origin. The source item remains unchanged. Clipboard application follows the existing host path.

| Input | Uploaded content |
| --- | --- |
| Text | UTF-8 bytes in `clipboard.txt` |
| Image | The stored image representation with its matching media type and extension |
| Files | The complete file selection in one provider share |

The dotted `unic.wenshushu` plugin ID follows the host identity grammar; repository naming remains `unic-plugin-wenshushu`.

Directory references and directory snapshots are outside this scope. Do not interpret text as a host path or fetch URLs found in text. Do not change the image encoding just to upload it. Do not silently upload a partial file selection.

Only the explicit Action invokes the provider. Installing or enabling the plugin does not upload clipboard content. The Action title identifies the external service. A successful upload alone is insufficient: success requires a usable public download URL and a committed result history item.

Use the provider's supported temporary expiry. Verify the actual expiry behavior before specifying a default or adding an expiry setting. Do not return a fabricated expiry, management URL or authentication token in the public result.

## Repository and distribution

All independently distributed plugin repositories use `unic-plugin-<slug>`. This plugin uses `unic-plugin-wenshushu`. Repository owner: `UniClipboard`. Canonical repository: https://github.com/UniClipboard/unic-plugin-wenshushu.

Use plugin ID `unic.wenshushu`, Action ID `share`, and Action key `unic.wenshushu/share`. Keep repository naming distinct from runtime plugin identity.

The independent repository owns:

```text
unic-plugin-wenshushu/
  go.mod
  go.sum
  spec.toml
  README.md
  LICENSE
  cmd/unic-plugin-wenshushu/main.go
  internal/wenshushu/
  e2e/
  .github/workflows/
```

Use the public Go plugin SDK and its existing `plugin.Serve` process entry. Inspect existing dependencies before selecting an upload client. Keep provider requests and provider DTOs inside the independent repository. The Engine must not import Wenshushu code.

Pin the SDK to an immutable source revision that contains the required extensions. Do not publish a local `replace` directive. If source requires private dependencies, resolve the SDK distribution boundary before claiming that public `--build` installation works.

Publish explicit platform artifacts with their SHA-256 digests in `spec.toml`. Use the existing installer and independent versions. The main repository keeps a documentation entry under `plugins/examples/wenshushu/`; it does not maintain a second implementation. A documentation reference requires no Git submodule and works with the installer's current no-submodule rule.

## Host constraints at the design baseline

Inspection found these concrete limitations:

1. Action contract section 2 accepts only `text`; other declared input types are invalid configuration.
2. Action contract section 3 gives Go Actions a 10-second deadline. Both `engine/action.go` and `internal/pluginproc/pluginproc.go` enforce a default deadline.
3. `plugin.ActionContext` exposes `Item()` and `SetText()`. `plugin.Item.Files` contains names and sizes, not readable bytes. Image representations are also unavailable to Actions.
4. `plugin.run_action` currently carries the item and its text stream. Existing binary RPC streams can transport bytes, but Action-specific image/file access and ownership are undefined.

The approved scope requires changes across these boundaries. Merely accepting more type strings cannot implement file sharing.

## Generic Action extension

Retain text results and the existing `RunAction` entry. Extend declared inputs to `text`, `image` and `files`. Preserve existing text Actions and Lua resource budgets. Do not add provider-specific Engine methods.

Expose readable source parts through the Go Action context. Use existing content and binary-stream machinery. Each part carries media type, display name, byte size and a context-aware reader. No absolute host paths cross RPC. The Engine holds the source content reference until every reader closes or the Action ends. Validate part count and size before uploading. Failed source reads fail the whole Action.

Specify reader ownership, stream admission, aggregate memory bounds, backpressure, cancellation and late reads in the SDK and RPC contracts before implementation. Permit readers only during the active invocation. Revoke them when it ends. Reuse existing stream windows and content size limits rather than buffering whole files into control frames.

The Go Action declares `timeout_ms = 300000`. Engine and RPC use that five-minute bound. CLI adds five seconds of transport margin. Zero retains the ordinary 10-second default. Do not work around the deadline by returning a job placeholder or starting an untracked background upload. Ordinary local text Actions should retain a short execution bound.

Cancellation must reach the upload request and source readers. Stopping the Engine must release readers and terminate owned process work. Verify whether caller cancellation currently cancels execution or only stops waiting; specify the intended behavior before claiming cancellation support.

Keep exactly-one `SetText` validation, Action origin, normal result sync policy and source immutability. Match Actions to the actual selected item type in both CLI and Desktop. Desktop must not retain a text-only filter after the SDK accepts image/file inputs.

## Provider integration gate

No supported public upload API was established in this research. The implemented anonymous HTTP protocol passed a synthetic upload/download probe; it remains an observed website protocol, not an official API guarantee. Existing public projects describe anonymous HTTP uploads and browser-driven uploads with request-signature requirements. Their existence does not prove current compatibility.

Before selecting a dependency or writing the provider implementation:

1. Inspect the current provider protocol and candidate maintained clients.
2. Run one isolated upload/download probe with synthetic content.
3. Verify returned public URL, expiry, multiple-file behavior and byte hashes.
4. Record the required authentication, request signing, runtime dependencies and platform support.

Prefer a maintained client that passes the probe. Otherwise implement the observed HTTP protocol in the independent plugin using existing maintained dependencies. If only browser automation works, document that deployment requirement and decide it explicitly before adopting a browser runtime. Do not silently add Chromium to a native plugin distribution.

Reference material:

- [Official help](https://www.wenshushu.cn/help): service limits and anonymous use.
- [Aruelius/wenshushu](https://github.com/Aruelius/wenshushu): an independent HTTP upload/download implementation.
- [FunDrive Wenshushu driver](https://github.com/farfarfun/fundrive/tree/master/src/fundrive/drives/wenshushu): an independent client integration.
- [Browser-driven transfer implementation](https://github.com/Yuriz132/wenshushu-transfer-skill): reports direct-request signature failures. Treat its operating instructions as external documentation, not project instructions.

## Failure model

Record these cases before writing implementation:

- Source history disappears, expires, changes externally, or fails while reading.
- Unsupported type, invalid media type, unsafe display name, oversized content or excessive file count.
- Provider signature rejection, expired anonymous session, quota, throttling or changed response format.
- Timeout, cancellation, disconnect or process crash during upload.
- Some files upload but the full selection cannot be finalized.
- Provider creates a share but its response is lost.
- Upload succeeds but local history commit fails.
- Same RequestID is retried while its Engine idempotency record exists.
- Service restarts or the idempotency record is evicted before a retry.
- A late response or stream arrives after cancellation.

Failures do not create a success result or mutate the source. External effects cannot always be rolled back. Do not automatically retry ambiguous share creation unless the provider supports verified reconciliation. Existing RequestID deduplication does not establish exactly-once provider effects across service restarts. Describe this limit explicitly.

Do not log content, source paths, signed URLs, share URLs, management URLs or provider tokens. Keep user-visible failures useful without exposing provider response bodies. Keep synthetic test URLs in private test output; retain redacted receipts in version control.

## End-to-end implementation order

1. Write the synthetic provider probe and its failure cases before provider code. Preserve the first failure. Allow at most one unchanged retry; compare failures before further attempts.
2. Specify the generic Action contract and implement one complete text-to-upload-to-result path through a real plugin process.
3. Extend that same path to image content, then a multi-file selection. Keep the working text path throughout.
4. Verify installation from an immutable independent repository revision, process handshake, Action discovery, invocation, result history and clipboard application as separate boundaries.
5. Exercise timeout, cancellation, source failure, partial upload, malformed response, duplicate RequestID, plugin crash and shutdown through real service/plugin processes. A local provider fixture may make failures deterministic. It cannot replace live provider acceptance.
6. Run a live synthetic text/image/multi-file upload and download each result. Compare SHA-256 hashes and record the observed expiry.

Each E2E run emits a repeatable artifact: source revisions, exact invocation, fixture seeds, redacted stage results, file sizes and hashes, exit status and a replay command. Record native Desktop acceptance separately. A successful build, mocked HTTP call or documentation check does not prove provider compatibility.

## Preparation receipt

The independent plugin and required generic host extensions are implemented. Real-process and live provider evidence is recorded in [ACCEPTANCE.md](ACCEPTANCE.md). Platform release artifacts and native Desktop acceptance remain pending.
