#!/usr/bin/env python3
"""Fetch pinned CPU inference and tokenizer binaries for the optional adapter."""

import hashlib
import io
from pathlib import Path
import sys
import tarfile
import urllib.request

VERSIONS = {"runtime": "1.30.0", "tokenizers": "1.26.0"}
PLATFORMS = {
    "amd64": {
        "runtime": ("x64", "a5ed5a3cac51fbb2e90da632ae43d19212faaa20e76484e62bcb7c23ddb3b3fd"),
        "tokenizers": ("amd64", "0713ac926d8473440572f59369a1bf2d3aa3fa5c6446eab45191f6e5f04ffeb0"),
    },
    "arm64": {
        "runtime": ("aarch64", "e16a27a8ed330bbc698df7330b0cf56e722f354e3bcc92118682c74ef3c3e3da"),
        "tokenizers": ("arm64", "e8a19c39bbad5b86044d98519b131806cad6b4a3e1408c11ad555b630051e49b"),
    },
}


def fetch_archive(url, expected):
    with urllib.request.urlopen(url, timeout=120) as response:
        payload = response.read()
    if hashlib.sha256(payload).hexdigest() != expected:
        raise RuntimeError(f"checksum mismatch for {url}")
    return tarfile.open(fileobj=io.BytesIO(payload), mode="r:gz")


def main():
    architecture, target = sys.argv[1], Path(sys.argv[2])
    selected = PLATFORMS[architecture]
    target.mkdir(parents=True, exist_ok=True)
    runtime_architecture, runtime_hash = selected["runtime"]
    runtime_version = VERSIONS["runtime"]
    runtime_name = f"onnxruntime-linux-{runtime_architecture}-{runtime_version}"
    runtime_url = f"https://github.com/microsoft/onnxruntime/releases/download/v{runtime_version}/{runtime_name}.tgz"
    with fetch_archive(runtime_url, runtime_hash) as archive:
        for member in archive.getmembers():
            name = Path(member.name).name
            if not member.isfile() or name not in {f"libonnxruntime.so.{runtime_version}", "LICENSE", "ThirdPartyNotices.txt"}:
                continue
            output = target / (name if name.startswith("libonnxruntime") else f"onnxruntime-{name}")
            with archive.extractfile(member) as source:
                output.write_bytes(source.read())
    tokenizer_architecture, tokenizer_hash = selected["tokenizers"]
    tokenizer_version = VERSIONS["tokenizers"]
    tokenizer_url = f"https://github.com/daulet/tokenizers/releases/download/v{tokenizer_version}/libtokenizers.linux-{tokenizer_architecture}.tar.gz"
    with fetch_archive(tokenizer_url, tokenizer_hash) as archive:
        with archive.extractfile("libtokenizers.a") as source:
            (target / "libtokenizers.a").write_bytes(source.read())
    if not (target / f"libonnxruntime.so.{runtime_version}").is_file():
        raise RuntimeError("native runtime archive did not contain the shared library")
    print(f"verified native runtime and tokenizer for {architecture}", flush=True)


if __name__ == "__main__":
    main()
