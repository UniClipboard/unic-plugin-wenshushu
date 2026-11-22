# Agent instructions

## Scope

This is the independent `UniClipboard/unic-plugin-wenshushu` repository.
Read README.md and DESIGN.md before implementation. Keep Wenshushu protocol code
here. Generic UniClipboard Engine, plugin SDK, RPC and host changes belong in
`UniClipboard/UniClipboard`.

## Working rules

- Use Simplified Chinese in conversation and English in documentation.
- Keep changes simple and modular. Inspect existing dependencies before adding one.
- Do not write unit tests after implementation. Prefer E2E as the test mechanism.
- Write failure cases before isolated implementation or provider probes.
- First complete a minimal text-to-upload-to-history path. Extend that working path.
- Every E2E run must emit a verifiable, repeatable artifact.
- Preserve the first failure. Set a time or run limit before retrying.
- Distinguish build checks, process E2E, live provider acceptance and native UI acceptance.
- Use synthetic content for provider probes. Do not upload user clipboard history.
- Do not log content, host paths, share URLs, management URLs or authentication data.
- Pin SDK revisions. Never publish local absolute-path dependency overrides.
- Do not claim installation or upload support until executed evidence exists.
- Public contracts require an explicit behavior decision before implementation changes.

## Identity

Repository naming: `unic-plugin-<slug>`.
Plugin ID: `wenshushu`. Action ID: `share`. Action key: `wenshushu/share`.
The approved input scope is text, images and files. Directory inputs are excluded.
