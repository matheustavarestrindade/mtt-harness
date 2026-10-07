#!/usr/bin/env python3
"""Fetch the pinned text-only model at image build time; verify every asset."""

import hashlib
from pathlib import Path
import sys
import urllib.request

REVISION = "daa72c51243991dfcaf9f9137d2c573d8f7790c0"
BASE = f"https://huggingface.co/onnx-community/embeddinggemma-2-ONNX/resolve/{REVISION}"
ASSETS = {
    "LICENSE": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
    "onnx/model_quantized.onnx": "d06edd601f851c633a2519304cbeb8dc6170d7ceb61b436625c17fb9b6e74953",
    "onnx/model_quantized.onnx_data": "278a7ff1248c3618e4bd11a607fc54f7bdc7778854230f3956d3f86bd9db4f3b",
    "tokenizer.json": "4d777ef5bdc1aa36227abdfb77c3e49e7b9c892d16e1b6bda41c393504828be4",
    "tokenizer_config.json": "17bd5d6e9364ca49a534e1502076593317c298d4a663623091ed45388f004874",
    "config.json": "8d011bfe08b5e345bbe0b81e5c6fd02c381920b345b986047bc2a33ce7b90d1d",
    "config_sentence_transformers.json": "031e56a498d33c349ab489a21885bcfe25b4fcba841149dc99e1e90d4a7c28f5",
}


def main():
    destination = Path(sys.argv[1])
    for name, expected in ASSETS.items():
        output = destination / name
        output.parent.mkdir(parents=True, exist_ok=True)
        temporary = output.with_suffix(output.suffix + ".download")
        digest = hashlib.sha256()
        url = "https://www.apache.org/licenses/LICENSE-2.0.txt" if name == "LICENSE" else f"{BASE}/{name}"
        with urllib.request.urlopen(url, timeout=120) as response:
            with temporary.open("wb") as stream:
                while chunk := response.read(1024 * 1024):
                    digest.update(chunk)
                    stream.write(chunk)
        if digest.hexdigest() != expected:
            temporary.unlink()
            raise RuntimeError(f"checksum mismatch for {name}")
        temporary.replace(output)
        print(f"verified {name}", flush=True)
    (destination / "REVISION").write_text(REVISION + "\n")


if __name__ == "__main__":
    main()
