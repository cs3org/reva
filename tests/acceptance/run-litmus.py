#!/usr/bin/env python3
"""
Run litmus WebDAV compliance tests against reva.

Replaces the litmus-ocis-old-webdav, litmus-ocis-new-webdav, and
litmus-owncloud-spaces-dav Drone pipelines.

Usage:
    python3 tests/acceptance/run-litmus.py --endpoint old-webdav
    python3 tests/acceptance/run-litmus.py --endpoint new-webdav
    python3 tests/acceptance/run-litmus.py --endpoint spaces-dav
"""

import argparse
import os
import signal
import socket
import subprocess
import sys
import tempfile
import time
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
DRONE_CONFIG_DIR = REPO_ROOT / "tests" / "oc-integration-tests" / "drone"
REVAD_BIN = REPO_ROOT / "cmd" / "revad" / "revad"

LITMUS_IMAGE = "owncloud/litmus:latest"
LITMUS_USERNAME = "einstein"
LITMUS_PASSWORD = "relativity"
LITMUS_TESTS = "basic http copymove props"

# einstein user UUID (see https://github.com/owncloud/ocis-accounts/blob/8de0530f31ed5ffb0bbb7f7f3471f87f429cb2ea/pkg/service/v0/service.go#L45)
EINSTEIN_UUID = "4c510ada-c86b-4815-8820-42cdf82c3d51"

# Revad configs for litmus tests (same set used by all three endpoints)
REVAD_CONFIGS = [
    "frontend.toml",
    "gateway.toml",
    "storage-users-ocis.toml",
    "storage-shares.toml",
    "storage-publiclink.toml",
    "shares.toml",
    "permissions-ocis-ci.toml",
    "users.toml",
]

FRONTEND_PORT = 20080
GATEWAY_PORT = 19000


def wait_for_port(port, timeout=60, label="service"):
    """Poll until a TCP port is accepting connections."""
    start = time.time()
    while time.time() - start < timeout:
        try:
            with socket.create_connection(("localhost", port), timeout=1):
                print(f"  {label} ready on port {port}")
                return
        except OSError:
            time.sleep(0.5)
    print(f"Timeout waiting for {label} on port {port} after {timeout}s", file=sys.stderr)
    sys.exit(1)


def get_docker_bridge_ip():
    """Get the Docker bridge gateway IP for container-to-host communication."""
    r = subprocess.run(
        ["docker", "network", "inspect", "bridge",
         "--format", "{{range .IPAM.Config}}{{.Gateway}}{{end}}"],
        capture_output=True, text=True, check=True,
    )
    return r.stdout.strip()


def prepare_configs():
    config_dir = Path(tempfile.mkdtemp(prefix="reva-ci-config-"))
    (REPO_ROOT / "tmp" / "reva" / "data").mkdir(parents=True, exist_ok=True)
    replacements = {
        "/drone/src": str(REPO_ROOT),
        "revad-services": "localhost",
    }
    for name in os.listdir(DRONE_CONFIG_DIR):
        src = DRONE_CONFIG_DIR / name
        dst = config_dir / name
        if src.is_file():
            content = src.read_text()
            for old, new in replacements.items():
                content = content.replace(old, new)
            dst.write_text(content)
    return config_dir


def start_revad_services(config_dir):
    """Start all revad processes, return list of Popen objects."""
    procs = []
    for config_name in REVAD_CONFIGS:
        config_path = config_dir / config_name
        p = subprocess.Popen(
            [str(REVAD_BIN), "-c", str(config_path)],
            cwd=str(config_dir),
        )
        procs.append(p)
    return procs


def get_space_id(bridge_ip):
    """Get the personal space ID for einstein user via PROPFIND."""
    base = f"http://{bridge_ip}:{FRONTEND_PORT}"

    # Trigger home creation by accessing the files endpoint
    subprocess.run(
        ["curl", "-s", "-k", "-u", f"{LITMUS_USERNAME}:{LITMUS_PASSWORD}",
         "-I", f"{base}/remote.php/dav/files/{LITMUS_USERNAME}"],
        capture_output=True,
    )
    time.sleep(1)

    # Get space ID via PROPFIND
    r = subprocess.run(
        ["curl", "-XPROPFIND", "-s", "-k",
         "-u", f"{LITMUS_USERNAME}:{LITMUS_PASSWORD}",
         f"{base}/dav/files/{LITMUS_USERNAME}"],
        capture_output=True, text=True,
    )
    # Parse space ID from oc:spaceid element
    import re
    match = re.search(r"<oc:spaceid>([^<]+)</oc:spaceid>", r.stdout)
    if not match:
        # Fallback: try awk-style extraction
        for line in r.stdout.split("\n"):
            if "oc:spaceid" in line:
                parts = line.split("<")
                for part in parts:
                    if part.startswith("oc:spaceid>"):
                        return part.split(">")[1].split("<")[0]
        print(f"Could not extract space ID from PROPFIND response", file=sys.stderr)
        print(f"Response: {r.stdout[:500]}", file=sys.stderr)
        sys.exit(1)
    return match.group(1)


def run_litmus(endpoint_url, bridge_ip):
    """Run litmus Docker container against the given endpoint."""
    result = subprocess.run(
        ["docker", "run", "--rm", "--network", "host",
         "-e", f"LITMUS_URL={endpoint_url}",
         "-e", f"LITMUS_USERNAME={LITMUS_USERNAME}",
         "-e", f"LITMUS_PASSWORD={LITMUS_PASSWORD}",
         "-e", f"TESTS={LITMUS_TESTS}",
         LITMUS_IMAGE],
    )
    return result.returncode


def main():
    parser = argparse.ArgumentParser(description="Run litmus WebDAV tests against reva")
    parser.add_argument("--endpoint", required=True,
                        choices=["old-webdav", "new-webdav", "spaces-dav"],
                        help="Which litmus endpoint variant to test")
    args = parser.parse_args()

    procs = []

    def cleanup(*_):
        for p in procs:
            try:
                p.terminate()
            except Exception:
                pass
        # Wait briefly for processes to exit
        for p in procs:
            try:
                p.wait(timeout=5)
            except Exception:
                p.kill()

    signal.signal(signal.SIGTERM, cleanup)
    signal.signal(signal.SIGINT, cleanup)

    try:
        # Build revad
        print("Building revad...")
        subprocess.run(["make", "build-ci"], cwd=str(REPO_ROOT), check=True)

        # Prepare configs (rewrite /drone/src paths)
        config_dir = prepare_configs()
        print(f"Config dir: {config_dir}")

        # Start revad services
        print("Starting revad services...")
        procs = start_revad_services(config_dir)

        # Wait for frontend and gateway to be ready
        wait_for_port(FRONTEND_PORT, timeout=60, label="frontend")
        wait_for_port(GATEWAY_PORT, timeout=60, label="gateway")

        # Get Docker bridge IP for container-to-host communication
        bridge_ip = get_docker_bridge_ip()
        base_url = f"http://{bridge_ip}:{FRONTEND_PORT}"

        # Determine litmus URL based on endpoint type
        if args.endpoint == "old-webdav":
            litmus_url = f"{base_url}/remote.php/webdav"
        elif args.endpoint == "new-webdav":
            litmus_url = f"{base_url}/remote.php/dav/files/{EINSTEIN_UUID}"
        elif args.endpoint == "spaces-dav":
            space_id = get_space_id(bridge_ip)
            litmus_url = f"{base_url}/remote.php/dav/spaces/{space_id}"
            print(f"Space ID: {space_id}")

        print(f"Running litmus: {args.endpoint} -> {litmus_url}")
        rc = run_litmus(litmus_url, bridge_ip)
        sys.exit(rc)

    finally:
        cleanup()


if __name__ == "__main__":
    main()
