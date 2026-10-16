# Airbridge

**Big files. Small distance.** Share files between phones, tablets, and computers on the same local network, through a responsive browser UI.

One Mac or Linux computer hosts the room. Devices join using its local address and room code. Files stay on that host: no cloud service, account, external assets, or frontend build is required.

## Start sharing

Install Go 1.23 or later, then run from this repository:

```sh
make run
```

Or without Make:

```sh
go build -o bin/airbridge ./cmd/airbridge
./bin/airbridge
```

1. The terminal prints the host’s local addresses and a random room code.
2. Open `http://localhost:8080` on the host. On another device, open the printed LAN address, for example `http://192.168.1.13:8080`.
3. Enter a device name and the room code in the UI.
4. Choose or drop files. Once they finish uploading, open **Room files** on the receiving device and select **Download**.

Keep the host awake and the sender’s browser open. Android, iPhone/iPad, Windows, macOS, and Linux devices use a modern browser; hosting currently supports macOS and Linux. Physical-device testing is still needed; see [verification](docs/verification.md).

## What it includes

- Responsive desktop and phone UI, drag-and-drop, multiple file selection, and a searchable shared library.
- Live upload progress, approximate speed and remaining time, pause/resume, and bounded automatic retries.
- Resume after a browser reload or host restart: join again and select the **same original file in the same browser at the same host address**. Saved partial bytes are reused.
- 8 MiB chunks, two concurrent files per browser, and streamed disk writes. Files do not need to fit in RAM.
- Native browser downloads with HTTP byte ranges. Download resume depends on the receiving browser and a valid room session.
- Default 50 GiB per-file limit and 100 GiB room quota, both configurable.
- Durable file metadata, safe filenames, an exclusive host-directory lock, and room-code authentication.

Uploads go **sender → host disk**, then downloads go **host disk → receiver**. The full upload must finish before downloading. This design needs disk space on the host and is not direct device-to-device WebRTC.

## Host options

```sh
./bin/airbridge -listen :8080 -data ./data -name "Studio room"
./bin/airbridge -max-file 107374182400 -quota 536870912000
./bin/airbridge -listen 127.0.0.1:8080  # host browser only
```

| Option | Default | Purpose |
| --- | --- | --- |
| `-listen` | `:8080` | Bind address and port. The default listens on all interfaces. |
| `-data` | `./data` | Host storage directory, relative to the working directory. |
| `-name` | `My Airbridge` | Name shown in the UI. |
| `-max-file` | `53687091200` | Per-file limit in bytes: 50 GiB. |
| `-quota` | `107374182400` | Total reserved bytes: 100 GiB, including incomplete uploads. |

A fresh room code is generated on startup. Set `AIRBRIDGE_PIN` in the host environment for a stable code (at least six characters). Do not put the code in URLs or source control. Sessions last 24 hours and end when the host restarts. Files and incomplete uploads remain until explicitly removed.

**Trusted LAN only:** the default HTTP transport is not encrypted. The code controls access; it does not protect traffic from network observers. Every joined member can download or remove every room file. Do not expose the port to the internet. Use a trusted network or a separately configured encrypted tunnel when necessary.

## Speed and recovery

Use wired Ethernet for the host where possible, or strong 5/6 GHz Wi-Fi. Both the host disk and the slowest network link affect throughput. Guest Wi-Fi isolation, VPN routes, or a firewall can prevent devices from reaching each other. Allow the selected port only on your trusted LAN; this app does not change firewall rules.

Pause before switching networks. Mobile operating systems can suspend background tabs. After an interruption, retry or select the same file again. The browser records the upload ID and checks the file’s name, size, modification time, and SHA-256 of its first/last 64 KiB samples. This guards against accidental reselection; it is **not a checksum of the entire file**. Changing the host address or clearing browser storage loses this resume mapping. Incomplete files remain visible for removal from Room files.

The quota reserves the whole declared size. Actual free disk space can be lower; disk errors stop the upload and appear in the UI. Remove unused files through Room files. Do not manually edit the host’s `.part`, `.blob`, or `.json` files. Back up the data directory while the host is stopped if you need to preserve the room.

## Development and verification

The runtime uses only Go’s standard library and native browser APIs. UI files are embedded in the binary; **rebuild and restart after editing them**.

```sh
make check                         # race tests, go vet, JavaScript syntax
make build                         # current host binary
make release                       # Linux/macOS, x64/ARM64 binaries in bin/
python3 scripts/benchmark.py --mib 1024  # disposable loopback test + SHA-256
```

Optional browser QA requires Node 20+ and Playwright:

```sh
npm install --no-save --package-lock=false playwright@1.62.1
npx playwright install chromium
make browser-test
# Or use system Chrome:
CHROME_PATH=/usr/bin/google-chrome make browser-test
```

The browser test starts its own host on a temporary port, uses a temporary data directory, checks independent sending/receiving sessions, and cleans up afterward. Screenshots go to `test-results/`. The benchmark also uses a disposable room. Neither test touches your normal `data/` directory.

See [architecture and protocol](docs/architecture.md) and [verification results](docs/verification.md).
