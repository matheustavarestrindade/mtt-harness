"""Atomic, checksum-verified build assets shared by Docker and native builds."""

import hashlib
import os
from pathlib import Path
import urllib.request


def file_digest(path):
    digest = hashlib.sha256()
    with Path(path).open("rb") as stream:
        while chunk := stream.read(1024 * 1024):
            digest.update(chunk)
    return digest.hexdigest()


def download_verified(url, destination, expected):
    destination = Path(destination)
    if destination.is_file() and file_digest(destination) == expected:
        print(f"verified cached {destination.name}", flush=True)
        return destination
    destination.parent.mkdir(parents=True, exist_ok=True)
    temporary = destination.with_name(destination.name + f".download-{os.getpid()}")
    try:
        digest = hashlib.sha256()
        with urllib.request.urlopen(url, timeout=120) as response, temporary.open("wb") as output:
            while chunk := response.read(1024 * 1024):
                digest.update(chunk)
                output.write(chunk)
        if digest.hexdigest() != expected:
            raise RuntimeError(f"checksum mismatch for {destination.name}")
        temporary.replace(destination)
    finally:
        temporary.unlink(missing_ok=True)
    print(f"verified {destination.name}", flush=True)
    return destination


def write_asset(destination, contents):
    destination = Path(destination)
    if destination.is_file() and file_digest(destination) == hashlib.sha256(contents).hexdigest():
        return
    destination.parent.mkdir(parents=True, exist_ok=True)
    temporary = destination.with_name(destination.name + f".extract-{os.getpid()}")
    try:
        temporary.write_bytes(contents)
        temporary.replace(destination)
    finally:
        temporary.unlink(missing_ok=True)
