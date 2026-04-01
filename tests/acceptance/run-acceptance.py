#!/usr/bin/env python3
"""
Run PHP Behat acceptance tests against reva with different storage backends.

Replaces the ocis-integration-tests, s3ng-integration-tests, and
posixfs-integration-tests Drone pipelines.

Usage:
    python3 tests/acceptance/run-acceptance.py --storage ocis --total-parts 4 --run-part 1
    python3 tests/acceptance/run-acceptance.py --storage s3ng --total-parts 6 --run-part 3
    python3 tests/acceptance/run-acceptance.py --storage posixfs --total-parts 4 --run-part 2
"""

import argparse
import os
import signal
import socket
import subprocess
import sys
import time
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
DRONE_CONFIG_DIR = REPO_ROOT / "tests" / "oc-integration-tests" / "drone"
DRONE_ENV_FILE = REPO_ROOT / ".drone.env"
REVAD_BIN = REPO_ROOT / "cmd" / "revad" / "revad"

FRONTEND_PORT = 20080
GATEWAY_PORT = 19000
LDAP_PORT = 636
REDIS_PORT = 6379
CEPH_PORT = 8080

# Container images
LDAP_IMAGE = "osixia/openldap:1.3.0"
REDIS_IMAGE = "redis:6-alpine"
CEPH_IMAGE = "ceph/daemon"
PHP_IMAGE = "owncloudci/php:8.4"

# Revad configs per storage backend
# Matches the exact configs from .drone.star for each storage type
REVAD_CONFIGS = {
    "ocis": [
        "frontend.toml",
        "gateway.toml",
        "shares.toml",
        "storage-shares.toml",
        "machine-auth.toml",
        "storage-users-ocis.toml",
        "storage-publiclink.toml",
        "permissions-ocis-ci.toml",
        "ldap-users.toml",
    ],
    "s3ng": [
        "frontend.toml",
        "gateway.toml",
        "shares.toml",
        "storage-users-s3ng.toml",
        "storage-publiclink.toml",
        "storage-shares.toml",
        "ldap-users.toml",
        "permissions-ocis-ci.toml",
        "machine-auth.toml",
    ],
    "posixfs": [
        "frontend.toml",
        "gateway.toml",
        "shares.toml",
        "storage-shares.toml",
        "machine-auth.toml",
        "storage-users-posixfs.toml",
        "storage-publiclink.toml",
        "permissions-ocis-ci.toml",
        "ldap-users.toml",
    ],
}

# Storage driver names as expected by the test framework
STORAGE_DRIVER_MAP = {
    "ocis": "OCIS",
    "s3ng": "S3NG",
    "posixfs": "POSIX",
}

# Expected failures files per storage backend
EXPECTED_FAILURES_MAP = {
    "ocis": "tests/acceptance/expected-failures-on-OCIS-storage.md",
    "s3ng": "tests/acceptance/expected-failures-on-S3NG-storage.md",
    "posixfs": "tests/acceptance/expected-failures-on-POSIX-storage.md",
}

# DELETE_USER_DATA_CMD per storage backend (paths rewritten at runtime)
DELETE_CMD_MAP = {
    "ocis": "rm -rf {data}/spaces/* {data}/blobs/* {data}/indexes/by-type/*",
    "s3ng": "rm -rf {data}/spaces/* {data}/blobs/* {data}/indexes/by-type/*",
    "posixfs": "bash -cx 'rm -rf {data}/users/* {data}/indexes/by-type/*'",
}

# Services required per storage backend
SERVICES_NEEDED = {
    "ocis": ["ldap"],
    "s3ng": ["ldap", "ceph"],
    "posixfs": ["redis", "ldap"],
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



def parse_drone_env():
    """Parse .drone.env file into a dict."""
    env = {}
    if DRONE_ENV_FILE.exists():
        for line in DRONE_ENV_FILE.read_text().splitlines():
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                key, value = line.split("=", 1)
                env[key.strip()] = value.strip()
    return env


def prepare_configs(storage):
    """Copy drone configs to a temp dir, rewriting hostnames and paths for GHA."""
    import tempfile

    config_dir = Path(tempfile.mkdtemp(prefix="reva-ci-config-"))
    data_dir = REPO_ROOT / "tmp" / "reva" / "data"
    data_dir.mkdir(parents=True, exist_ok=True)

    # Hostname/path replacements: drone container DNS -> localhost
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


def start_ldap():
    """Start LDAP service container."""
    print("Starting LDAP...")
    subprocess.run(
        ["docker", "run", "-d", "--name", "ldap", "--network", "host",
         "-e", "LDAP_DOMAIN=owncloud.com",
         "-e", "LDAP_ORGANISATION=ownCloud",
         "-e", "LDAP_ADMIN_PASSWORD=admin",
         "-e", "LDAP_TLS_VERIFY_CLIENT=never",
         "-e", "HOSTNAME=ldap",
         LDAP_IMAGE],
        check=True,
    )
    wait_for_port(LDAP_PORT, timeout=60, label="LDAP")


def start_redis():
    """Start Redis service container."""
    print("Starting Redis...")
    subprocess.run(
        ["docker", "run", "-d", "--name", "redis", "--network", "host",
         "-e", "REDIS_DATABASES=1",
         REDIS_IMAGE],
        check=True,
    )
    wait_for_port(REDIS_PORT, timeout=30, label="Redis")


def start_ceph():
    print("Starting Ceph...")
    subprocess.run(
        ["docker", "run", "-d", "--name", "ceph",
         "-p", f"{CEPH_PORT}:{CEPH_PORT}",
         "-e", "CEPH_DAEMON=demo",
         "-e", "NETWORK_AUTO_DETECT=1",
         "-e", "MON_IP=0.0.0.0",
         "-e", "CEPH_PUBLIC_NETWORK=0.0.0.0/0",
         "-e", f"RGW_CIVETWEB_PORT={CEPH_PORT}",
         "-e", "RGW_NAME=ceph",
         "-e", "CEPH_DEMO_UID=test-user",
         "-e", "CEPH_DEMO_ACCESS_KEY=test",
         "-e", "CEPH_DEMO_SECRET_KEY=test",
         "-e", "CEPH_DEMO_BUCKET=test",
         CEPH_IMAGE],
        check=True,
    )
    wait_for_port(CEPH_PORT, timeout=180, label="Ceph RGW")
    # Wait for demo bucket creation after RGW starts accepting connections
    time.sleep(15)


SERVICE_STARTERS = {
    "ldap": start_ldap,
    "redis": start_redis,
    "ceph": start_ceph,
}


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


def clone_test_repos():
    """Clone the ocis testing repo at the pinned commit (mirrors cloneApiTestReposStep)."""
    drone_env = parse_drone_env()
    commit_id = drone_env.get("APITESTS_COMMITID", "")
    branch = drone_env.get("APITESTS_BRANCH", "master")
    repo_url = drone_env.get("APITESTS_REPO_GIT_URL", "https://github.com/owncloud/ocis.git")

    testing_dir = REPO_ROOT / "tmp" / "testing"
    testrunner_dir = REPO_ROOT / "tmp" / "testrunner"

    if not testing_dir.exists():
        print("Cloning testing repo...")
        subprocess.run(
            ["git", "clone", "-b", "master", "--depth=1",
             "https://github.com/owncloud/testing.git", str(testing_dir)],
            check=True,
        )

    if not testrunner_dir.exists():
        print(f"Cloning test runner (branch={branch}, commit={commit_id})...")
        subprocess.run(
            ["git", "clone", "-b", branch, "--single-branch", "--no-tags",
             repo_url, str(testrunner_dir)],
            check=True,
        )
        if commit_id:
            subprocess.run(
                ["git", "checkout", commit_id],
                cwd=str(testrunner_dir),
                check=True,
            )


def run_acceptance_tests(storage, total_parts, run_part):
    """Run PHP Behat acceptance tests via make test-acceptance-api."""
    data_dir = str(REPO_ROOT / "tmp" / "reva" / "data")
    testrunner_dir = REPO_ROOT / "tmp" / "testrunner"
    expected_failures = str(REPO_ROOT / EXPECTED_FAILURES_MAP[storage])

    delete_cmd = DELETE_CMD_MAP[storage].format(data=data_dir)

    env = {
        **os.environ,
        "TEST_SERVER_URL": f"http://localhost:{FRONTEND_PORT}",
        "OCIS_REVA_DATA_ROOT": f"{data_dir}/",
        "DELETE_USER_DATA_CMD": delete_cmd,
        "STORAGE_DRIVER": STORAGE_DRIVER_MAP[storage],
        "TEST_WITH_LDAP": "true",
        "REVA_LDAP_HOSTNAME": "localhost",
        "TEST_REVA": "true",
        "BEHAT_FILTER_TAGS": "~@skip&&~@skipOnReva&&~@env-config",
        "DIVIDE_INTO_NUM_PARTS": str(total_parts),
        "RUN_PART": str(run_part),
        "EXPECTED_FAILURES_FILE": expected_failures,
        "REGULAR_USER_PASSWORD": "relativity",
        "ACCEPTANCE_TEST_TYPE": "core-api",
    }

    print(f"Running acceptance tests: storage={storage}, part={run_part}/{total_parts}")
    result = subprocess.run(
        ["make", "test-acceptance-api"],
        cwd=str(testrunner_dir),
        env=env,
    )
    return result.returncode


def main():
    parser = argparse.ArgumentParser(description="Run reva acceptance tests")
    parser.add_argument("--storage", required=True,
                        choices=["ocis", "s3ng", "posixfs"],
                        help="Storage backend to test")
    parser.add_argument("--total-parts", type=int, required=True,
                        help="Total number of parallel shards")
    parser.add_argument("--run-part", type=int, required=True,
                        help="Which shard to run (1-based)")
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

        # Start required services
        for service in SERVICES_NEEDED[args.storage]:
            SERVICE_STARTERS[service]()
            docker_containers.append(service)

        # Prepare configs
        config_dir = prepare_configs(args.storage)
        print(f"Config dir: {config_dir}")

        # Start revad services
        print(f"Starting revad services ({args.storage})...")
        procs = start_revad_services(config_dir, args.storage)

        # Wait for frontend and gateway to be ready
        wait_for_port(FRONTEND_PORT, timeout=60, label="frontend")
        wait_for_port(GATEWAY_PORT, timeout=60, label="gateway")

        # Clone test repos
        clone_test_repos()

        # Run acceptance tests
        rc = run_acceptance_tests(args.storage, args.total_parts, args.run_part)
        sys.exit(rc)

    finally:
        cleanup()


if __name__ == "__main__":
    main()
