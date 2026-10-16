# Architecture and transfer protocol

Airbridge is a single Go process with embedded HTML/CSS/JavaScript. There is no JavaScript build pipeline, database service, cloud signaling, or external asset request.

```text
Sender browser ── 8 MiB PATCH chunks ──> Host .part file
                                            │ complete + sync + rename
                                            ▼
Receiver browser <── streamed HTTP GET ── Host .blob file
```

## Upload lifecycle

1. `POST /api/join` exchanges a room code and device name for an expiring HttpOnly, SameSite=Strict cookie.
2. `POST /api/files` reserves quota and creates a random-ID `.part` file and atomic JSON metadata.
3. `PATCH /api/files/{id}` appends a bounded binary chunk with an exact `Upload-Offset` and `Content-Length`. A per-file write mutex serializes body mutations. A separate short metadata lock keeps library reads responsive during slow uploads. A short request truncates back to the previous offset. An offset conflict returns 409; clients query the current offset before retrying.
4. `GET /api/files/{id}` returns the acknowledged offset. A lost chunk response is reconciled here rather than blindly sending the same bytes twice.
5. `POST /api/files/{id}/complete` requires offset = declared size. The host syncs and renames the body to `.blob` before publishing it. Completion is idempotent. The host recognizes completed bodies after a restart even if its metadata update was interrupted.
6. `GET /api/files/{id}/download` returns a forced attachment using `http.ServeContent`. HEAD and byte ranges are supported. Incomplete bodies cannot be downloaded.
7. `DELETE /api/files/{id}` records removal intent with a tombstone, deletes the body, and releases quota. Startup finishes interrupted deletions.

`GET /api/files` lists the room and its reserved storage. `GET /api/room` supplies the room name, limits, and candidate LAN addresses. `POST /api/leave` revokes the session. `POST /api/fingerprint` hashes at most 128 KiB of file samples for browser resume identity on HTTP origins.

## State and durability

- The browser holds File references and a bounded map of resume IDs in localStorage. It never saves the room code there.
- Upload identity uses name, size, modification time, and sampled SHA-256. It detects common accidental mismatches without reading the full original file. It does not prove complete-file integrity.
- Bodies and metadata use random IDs, not user filenames. Filenames with separators or control characters are rejected; Unicode and HTML-like characters are displayed as plain text.
- On restart, the host derives offsets from actual body lengths. A process crash may leave a partial chunk; its saved prefix is resumable.
- Successful completion syncs file contents. This is not a transactional filesystem or a promise against hardware failure or sudden power loss. Keep important originals until the receiver has verified them.
- A Linux/macOS advisory file lock prevents two hosts from opening the same data directory. The OS releases it after a crash. The lock file is intentionally never unlinked.
- Room sessions are memory-only. A restart invalidates cookies but preserves files and their IDs.

## Resource limits

File bodies stream through bounded buffers rather than whole-file allocations. The browser uses two upload slots and slices files into 8 MiB chunks. Metadata requests are limited to 8 KiB; fingerprint requests to 128 KiB. The room allows up to 10,000 entries, 128 live sessions, and a configurable total reservation quota. Failed room-code attempts are limited per source address.

Read-header and per-request deadlines bound slow clients. Graceful shutdown waits up to 15 seconds before closing remaining connections; interrupted uploads can resume. Downloads have no arbitrary whole-file write timeout.

## Access model

The shared room is the access boundary. Everyone with a room session can view, upload, resume through the API, download, and remove room files. No per-recipient inbox or ownership policy is implied.

The host rejects foreign origins, cross-site browser requests, and unexpected DNS hostnames. Cookies are HttpOnly and SameSite=Strict. Static pages use a restrictive Content Security Policy. These measures reduce browser attacks; they do not encrypt HTTP or turn a LAN room into an internet-facing service.

## Source map

- `cmd/airbridge`: host flags, room-code generation, and shutdown.
- `internal/server`: request routing, sessions, validation, upload/download handlers.
- `internal/store`: quota reservations, disk I/O, recovery, directory locking.
- `internal/web/assets`: buildless responsive UI, transfer queue, shared library.
- `scripts`: disposable browser tests and loopback benchmark.
