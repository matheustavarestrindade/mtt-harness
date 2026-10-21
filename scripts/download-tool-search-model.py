#!/usr/bin/env python3
"""Fetch the pinned MiniLM assets for Docker or a native build."""

from pathlib import Path
import sys

from verified_download import download_verified, write_asset

REVISION = "1110a243fdf4706b3f48f1d95db1a4f5529b4d41"
BASE = f"https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/{REVISION}"
ASSETS = {
    "model.onnx": ("onnx/model.onnx", "6fd5d72fe4589f189f8ebc006442dbb529bb7ce38f8082112682524616046452"),
    "tokenizer.json": ("tokenizer.json", "be50c3628f2bf5bb5e3a7f17b1f74611b2561a3a27eeab05e5aa30f411572037"),
    "config.json": ("config.json", "953f9c0d463486b10a6871cc2fd59f223b2c70184f49815e7efbcab5d8908b41"),
    "sentence_bert_config.json": ("sentence_bert_config.json", "fc1993fde0a95c24ec6c022539d41cf6e2f7c9721e5415d6fb6897472a9cd4b7"),
}


def main():
    destination = Path(sys.argv[1])
    for name, (remote, expected) in ASSETS.items():
        download_verified(f"{BASE}/{remote}", destination / name, expected)
    # The pinned model card declares Apache-2.0; that revision has no LICENSE file.
    download_verified("https://www.apache.org/licenses/LICENSE-2.0.txt", destination / "LICENSE",
                      "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30")
    write_asset(destination / "REVISION", (REVISION + "\n").encode())


if __name__ == "__main__":
    main()
