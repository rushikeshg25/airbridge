# Changelog

## 1.0.0 — 2026-10-16

First public release of Airbridge, a browser-based LAN file-sharing room hosted on Linux or macOS.

- Responsive UI for sending files, monitoring transfers, and searching/downloading shared files.
- Streamed 8 MiB uploads, pause/resume, retries, and recovery after a host restart.
- Room-code access, expiring sessions, bounded storage, and streamed range downloads.
- Linux and macOS binaries for x64 and ARM64, bundled in archives with SHA-256 checksums.
- Fixed room-list stalls during slow uploads by separating body writes from metadata reads.
- Fixed restart recovery when data-directory names contain glob characters such as brackets.
- Fixed late file-selection and library responses continuing after logout.
- Added Linux/macOS Go checks and browser regression checks in GitHub Actions.

The default HTTP transport is intended for trusted local networks. All room members share access to every file. macOS downloads are unsigned and not notarized. Physical Android/iOS devices, Firefox, and Safari have not yet been verified.
