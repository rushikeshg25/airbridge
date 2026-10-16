#!/usr/bin/env python3
"""Measure a disposable local transfer and verify its complete SHA-256 digest.

Build bin/airbridge first. This loopback result is not a Wi-Fi benchmark.
"""
import argparse
import hashlib
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--mib', type=int, default=256)
    args = parser.parse_args()
    if args.mib < 1:
        parser.error('--mib must be positive')
    size = args.mib * 1024 * 1024
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        port = listener.getsockname()[1]
    base = f'http://127.0.0.1:{port}'
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

    def request(method, route, data=None, headers=None):
        if isinstance(data, dict):
            data = json.dumps(data).encode()
            headers = {'Content-Type': 'application/json', **(headers or {})}
        return opener.open(urllib.request.Request(base + route, data=data, headers=headers or {}, method=method), timeout=180)

    with tempfile.TemporaryDirectory(prefix='airbridge-bench-') as directory:
        pin = secrets.token_hex(16)
        process = subprocess.Popen(
            [str(Path('bin/airbridge').resolve()), '-listen', f'127.0.0.1:{port}', '-data', directory,
             '-max-file', str(size), '-quota', str(size * 2)],
            env={**os.environ, 'AIRBRIDGE_PIN': pin}, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE,
        )
        try:
            for _ in range(100):
                if process.poll() is not None:
                    raise RuntimeError('Host failed: ' + process.stderr.read().decode())
                try:
                    with request('GET', '/'):
                        break
                except urllib.error.URLError:
                    time.sleep(0.05)
            else:
                raise RuntimeError('Host did not start')
            with request('POST', '/api/join', {'pin': pin, 'name': 'Local benchmark'}):
                pass
            with request('POST', '/api/files', {'name': 'benchmark.bin', 'size': size}) as response:
                file_id = json.load(response)['id']
            route = '/api/files/' + file_id
            chunk = secrets.token_bytes(8 * 1024 * 1024)
            expected = hashlib.sha256()
            started = time.perf_counter()
            for offset in range(0, size, len(chunk)):
                block = chunk[:min(len(chunk), size - offset)]
                expected.update(block)
                with request('PATCH', route, block, {'Upload-Offset': str(offset), 'Content-Type': 'application/octet-stream'}):
                    pass
            with request('POST', route + '/complete', b''):
                pass
            upload_seconds = time.perf_counter() - started
            actual = hashlib.sha256()
            received = 0
            started = time.perf_counter()
            with request('GET', route + '/download') as response:
                while block := response.read(8 * 1024 * 1024):
                    received += len(block)
                    actual.update(block)
            download_seconds = time.perf_counter() - started
            assert received == size, (received, size)
            assert expected.digest() == actual.digest(), 'SHA-256 mismatch'
            with request('DELETE', route):
                pass
            print(json.dumps({
                'transport': 'loopback HTTP, not real-device LAN', 'mib': args.mib,
                'upload_seconds': round(upload_seconds, 3), 'download_seconds': round(download_seconds, 3),
                'upload_MiB_s': round(args.mib / upload_seconds, 1),
                'download_MiB_s': round(args.mib / download_seconds, 1), 'sha256_verified': True,
            }, indent=2))
        finally:
            process.terminate()
            try:
                process.wait(timeout=20)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
            process.stderr.close()


if __name__ == '__main__':
    main()
