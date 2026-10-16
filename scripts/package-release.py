#!/usr/bin/env python3
"""Package the four make release binaries with instructions and SHA-256 hashes."""
import argparse
import gzip
import hashlib
import io
from pathlib import Path
import re
import tarfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('version', help='semantic version such as 1.0.0')
    args = parser.parse_args()
    if not re.fullmatch(r'\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?', args.version):
        parser.error('version must be a semantic version, such as 1.0.0')
    root = Path(__file__).resolve().parent.parent
    output = root / 'bin' / 'releases'
    output.mkdir(parents=True, exist_ok=True)
    checksums = []
    for platform in ('linux-amd64', 'linux-arm64', 'macos-amd64', 'macos-arm64'):
        binary = root / 'bin' / f'airbridge-{platform}'
        if not binary.is_file():
            parser.error(f'missing {binary.name}; run make release first')
        name = f'airbridge_{args.version}_{platform}.tar.gz'
        # Normalize metadata and gzip timestamps for reproducible archives.
        with (output / name).open('wb') as raw:
            with gzip.GzipFile(filename='', mode='wb', fileobj=raw, mtime=0) as compressed:
                with tarfile.open(fileobj=compressed, mode='w') as archive:
                    members = [('airbridge', binary.read_bytes(), 0o755)]
                    documents = [root / 'README.md', root / 'CHANGELOG.md']
                    documents.extend(sorted(file for file in (root / 'docs').rglob('*') if file.is_file()))
                    members.extend((str(file.relative_to(root)), file.read_bytes(), 0o644) for file in documents)
                    for filename, data, mode in members:
                        info = tarfile.TarInfo(filename)
                        info.size = len(data)
                        info.mode = mode
                        archive.addfile(info, io.BytesIO(data))
        digest = hashlib.sha256((output / name).read_bytes()).hexdigest()
        checksums.append(f'{digest}  {name}')
    (output / 'SHA256SUMS').write_text('\n'.join(checksums) + '\n')
    print(f'Packaged four platforms and SHA256SUMS in {output}')


if __name__ == '__main__':
    main()
