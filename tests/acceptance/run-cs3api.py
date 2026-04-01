#!/usr/bin/env python3
"""
Run CS3 API validator against reva.

Replaces the cs3api-validator-ocis and cs3api-validator-S3NG Drone pipelines.

Usage:
    python3 tests/acceptance/run-cs3api.py --storage ocis
    python3 tests/acceptance/run-cs3api.py --storage s3ng
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

CS3API_VALIDATOR_IMAGE = "owncloud/cs3api-validator:0.2.1"

GATEWAY_PORT = 19000
FRONTEND_PORT = 20080

# Revad configs per storage backend
REVAD_CONFIGS = {
    "ocis": [
        "frontend.toml",
        "gateway.toml",
        "storage-users-ocis.toml",
        "storage-shares.toml",
        "storage-publiclink.toml",
        "shares.toml",
        "permissions-ocis-ci.toml",
        "users.toml",
    ],
    "s3ng": [
        "frontend.toml",
        "gateway.toml",
        "storage-users-s3ng.toml",
        "storage-shares.toml",
        "storage-publiclink.toml",
        "shares.toml",
        "permissions-ocis-ci.toml",
        "users.toml",
    ],
}

# Ceph service config (for S3NG)
CEPH_IMAGE = "ceph/daemon"
CEPH_PORT = 8080
CEPH_ENV = {
    "CEPH_DAEMON": "demo",
    "NETWORK_AUTO_DETECT": "1",
    "MON_IP": "0.0.0.0",
    "CEPH_PUBLIC_NETWORK": "0.0.0.0/0",
    "RGW_CIVETWEB_PORT": str(CEPH_PORT),
    "RGW_NAME": "ceph",
    "CEPH_DEMO_UID": "test-user",
    "CEPH_DEMO_ACCESS_KEY": "test",
    "CEPH_DEMO_SECRET_KEY": "test",
    "CEPH_DEMO_BUCKET": "test",
}


def wait_for_port(port, timeout=60, label="service"):
    start = time.time()
    while time.time() - start < timeout:
        try:
            with socket.create_connection(("localhost", port), timeout=1):
                return
        except OSError:
            time.sleep(0.5)
    sys.exit(f"Timeout waiting for {label} on port {port} after {timeout}s")



def get_docker_bridge_ip():
    """Get the Docker bridge gateway IP for container-to-host communication."""
    r = subprocess.run(
        ["docker", "network", "inspect", "bridge",
         "--format", "{{range .IPAM.Config}}{{.Gateway}}{{end}}"],
        capture_output=True, text=True, check=True,
    )
    return r.stdout.strip()


def prepare_configs(storage):
    config_dir = Path(tempfile.mkdtemp(prefix="reva-ci-config-"))
    (REPO_ROOT / "tmp" / "reva" / "data").mkdir(parents=True, exist_ok=True)
    replacements = {
        "/drone/src": str(REPO_ROOT),
        "revad-services": "localhost",
        "ldaps://ldap:": "ldaps://localhost:",
        "redis://redis:": "redis://localhost:",
        "http://ceph:": "http://localhost:",
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


def start_ceph():
    print("Starting Ceph...")
    env_args = []
    for k, v in CEPH_ENV.items():
        env_args.extend(["-e", f"{k}={v}"])

    subprocess.run(
        ["docker", "run", "-d", "--name", "ceph",
         "-p", f"{CEPH_PORT}:{CEPH_PORT}"] +
        env_args + [CEPH_IMAGE],
        check=True,
    )
    wait_for_port(CEPH_PORT, timeout=180, label="Ceph RGW")
    # Wait for demo bucket creation after RGW starts accepting connections
    time.sleep(15)


def start_revad_services(config_dir, storage):
    """Start all revad processes, return list of Popen objects."""
    procs = []
    for config_name in REVAD_CONFIGS[storage]:
        config_path = config_dir / config_name
        p = subprocess.Popen(
            [str(REVAD_BIN), "-c", str(config_path)],
            cwd=str(config_dir),
        )
        procs.append(p)
    return procs


def main():
    parser = argparse.ArgumentParser(description="Run CS3 API validator against reva")
    parser.add_argument("--storage", required=True, choices=["ocis", "s3ng"],
                        help="Storage backend to test")
    args = parser.parse_args()

    procs = []
    docker_containers = []

    def cleanup(*_):
        for p in procs:
            try:
                p.terminate()
            except Exception:
                pass
        for p in procs:
            try:
                p.wait(timeout=5)
            except Exception:
                p.kill()
        for name in docker_containers:
            subprocess.run(["docker", "rm", "-f", name], capture_output=True)

    signal.signal(signal.SIGTERM, cleanup)
    signal.signal(signal.SIGINT, cleanup)

    try:
        # Build revad
        print("Building revad...")
        subprocess.run(["make", "build-ci"], cwd=str(REPO_ROOT), check=True)

        # Start Ceph if S3NG
        if args.storage == "s3ng":
            start_ceph()
            docker_containers.append("ceph")

        # Prepare configs
        config_dir = prepare_configs(args.storage)
        print(f"Config dir: {config_dir}")

        # Start revad services
        print(f"Starting revad services ({args.storage})...")
        procs = start_revad_services(config_dir, args.storage)

        # Wait for gateway to be ready
        wait_for_port(GATEWAY_PORT, timeout=60, label="gateway")
        wait_for_port(FRONTEND_PORT, timeout=60, label="frontend")

        # Get bridge IP for validator container to reach host revad
        bridge_ip = get_docker_bridge_ip()

        # Run CS3 API validator
        print(f"Running CS3 API validator (storage={args.storage}, endpoint={bridge_ip}:{GATEWAY_PORT})...")
        result = subprocess.run(
            ["docker", "run", "--rm", "--network", "host",
             "--entrypoint", "/usr/bin/cs3api-validator",
             CS3API_VALIDATOR_IMAGE,
             "/var/lib/cs3api-validator",
             f"--endpoint={bridge_ip}:{GATEWAY_PORT}"],
        )
        sys.exit(result.returncode)

    finally:
        cleanup()


if __name__ == "__main__":
    main()
