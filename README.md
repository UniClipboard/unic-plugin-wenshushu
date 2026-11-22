# unic-plugin-wenshushu

A planned UniClipboard Action plugin for temporary sharing through Wenshushu.

**Status: design and development preparation. No executable plugin or release is available yet.**

## Intended behavior

Select one text, image or file history item and invoke “通过文叔叔临时分享”.
The plugin uploads the selected content to Wenshushu and returns its public
download URL. UniClipboard saves the URL as a new text history item and preserves
the source. Nothing uploads automatically.

- Repository: `UniClipboard/unic-plugin-wenshushu`
- Plugin ID: `wenshushu`
- Action ID: `share`
- Action key: `wenshushu/share`
- Distribution: an independent Go process plugin using the UniClipboard SDK

Independent plugin repositories follow the `unic-plugin-<slug>` naming convention.

## Implementation boundary

The approved scope includes text, images and files. The current host Action API
only supports text and has a short execution deadline. Generic source readers,
RPC transport and a bounded upload execution policy must be added to the host.
Provider compatibility must be verified with synthetic upload/download probes.

See [DESIGN.md](DESIGN.md) for the integration plan, failure model and E2E acceptance.
This repository owns provider-specific code and releases. The main UniClipboard
repository owns generic Engine, SDK, RPC, CLI and Desktop changes.

## Development order

1. Verify the current provider upload/download protocol with synthetic fixtures.
2. Implement one complete text Action through a real plugin process.
3. Extend the same path to images and multi-file selections.
4. Verify independent installation, result history, cancellation and failure paths.
5. Publish platform artifacts after live provider acceptance passes.

Do not install this repository as a plugin yet. It intentionally has no
`spec.toml`, executable or fabricated installation command.
