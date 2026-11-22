# Acceptance receipt

## Executed locally

- Deterministic provider HTTP E2E: passed.
- Real Engine and separate plugin process: text, PNG, multipart file and empty file passed.
- Repeated RequestID: same result history and one provider invocation.
- Provider HTTP failure: no success result; source history preserved.
- Live anonymous provider: text, valid PNG and a 2,400,000-byte binary shared in one batch.
- Independent anonymous recipient: all three files downloaded and compared byte for byte.
- Provider expiry: approximately 86,396 seconds remaining after completion.
- Native GUI acceptance: not run.
- Public release artifacts: not published.

## Live payload checksums

| Synthetic payload | SHA-256 |
| --- | --- |
| `clipboard.txt` | `ff77ff76ca5b25e3edc2ccd191c0d147d703b8c4a47227a73c3d86daf1476991` |
| `image.png` | `65da4c2ded5da4f624781ee71b51f8d679358e344314f20af7d05dae1a40a247` |
| `中文 文件.bin` | `d5a3cb1d418bb275d10f54b15978dfdf961b0179ed974437fb62db16ac1c9b8b` |

The private run artifact is `.cache/acceptance/run.lMd5MK/`. Run
`GOWORK=off WENSHUSHU_LIVE=1 sh scripts/verify.sh` to generate a new independent
receipt. This table records the executed run, not a guarantee of future provider
compatibility.

## First failures and fixes

1. `task/addsend` returned `user-agent error:-1`. The current frontend includes
   `Accept-Language`; adding that required header made task creation succeed.
2. Uploads without the `uploadv2/fast` admission step were not registered as
   downloadable files. Admission and numeric fast-status decoding fixed this.
3. One upload ID per file caused a three-file share to expose only one file.
   The current frontend uses one upload ID for the whole selection. Matching
   that batch scope and waiting for batch processing fixed the failure.
4. The first host process E2E used a symlinked macOS temporary path. The host
   correctly rejected it. Acceptance now resolves the profile path.

First failure logs remain in ignored `.cache/` directories. No user clipboard
history was uploaded. The live probes used synthetic fixtures only.

## Evidence boundaries

The real-process suite uses a deterministic local provider fixture. Live provider
round-trip acceptance separately exercises the production client. Native Desktop
presentation, real system clipboard application, very large uploads near the
provider quota, CAPTCHA challenges, account login and platform release packages
are not covered by this receipt.

## Dependency identity

The plugin pins the public SDK through the canonical remote repository at
`549ef96c8ab5dae7f5791d03a05a9726087de286`. A temporary local Go workspace was
used during initial development. Final independent verification passed with `GOWORK=off`, including live
upload/download and real-process E2E. No local filesystem replacement is shipped.
