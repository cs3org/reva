OC_CI_GOLANG = "owncloudci/golang:1.19"
OC_CI_ALPINE = "owncloudci/alpine:latest"
OSIXIA_OPEN_LDAP = "osixia/openldap:1.3.0"
OC_CI_PHP = "cs3org/behat:latest"
OC_CI_BAZEL_BUILDIFIER = "owncloudci/bazel-buildifier:latest"

def makeStep():
    return {
        "name": "build",
        "image": OC_CI_GOLANG,
        "commands": [
            "make revad",
        ],
    }

def cloneApiTestReposStep():
    return {
        "name": "clone-api-test-repos",
        "image": OC_CI_ALPINE,
        "commands": [
            "source /drone/src/.drone.env",
            "git clone -b master --depth=1 https://github.com/owncloud/testing.git /drone/src/tmp/testing",
            "git clone -b $APITESTS_BRANCH --single-branch --no-tags $APITESTS_REPO_GIT_URL /drone/src/tmp/testrunner",
            "cd /drone/src/tmp/testrunner",
            "git checkout $APITESTS_COMMITID",
        ],
    }

# Shared service definitions
def ldapService():
    return {
        "name": "ldap",
        "image": OSIXIA_OPEN_LDAP,
        "pull": "always",
        "environment": {
            "LDAP_DOMAIN": "owncloud.com",
            "LDAP_ORGANISATION": "ownCloud",
            "LDAP_ADMIN_PASSWORD": "admin",
            "LDAP_TLS_VERIFY_CLIENT": "never",
            "HOSTNAME": "ldap",
        },
    }

def cephService():
    return {
        "name": "ceph",
        "image": "ceph/daemon",
        "pull": "always",
        "environment": {
            "CEPH_DAEMON": "demo",
            "NETWORK_AUTO_DETECT": "4",
            "MON_IP": "0.0.0.0",
            "CEPH_PUBLIC_NETWORK": "0.0.0.0/0",
            "RGW_CIVETWEB_PORT": "4000 ",
            "RGW_NAME": "ceph",
            "CEPH_DEMO_UID": "test-user",
            "CEPH_DEMO_ACCESS_KEY": "test",
            "CEPH_DEMO_SECRET_KEY": "test",
            "CEPH_DEMO_BUCKET": "test",
        },
    }

# Pipeline definitions
def main(ctx):
    return [
        checkStarlark(),
        ocisIntegrationTest(),
    ]

def ocisIntegrationTest():
    return {
        "kind": "pipeline",
        "type": "docker",
        "name": "ocis-integration-tests-2",
        "platform": {
            "os": "linux",
            "arch": "amd64",
        },
        "trigger": {
            "event": {
                "include": [
                    "pull_request",
                    "tag",
                ],
            },
        },
        "steps": [
            makeStep(),
            {
                "name": "revad-services",
                "image": OC_CI_GOLANG,
                "detach": True,
                "commands": [
                    "cd /drone/src/tests/oc-integration-tests/drone/",
                    "/drone/src/cmd/revad/revad -c frontend.toml &",
                    "/drone/src/cmd/revad/revad -c gateway.toml &",
                    "/drone/src/cmd/revad/revad -c shares.toml &",
                    "/drone/src/cmd/revad/revad -c storage-home-ocis.toml &",
                    "/drone/src/cmd/revad/revad -c storage-users-ocis.toml &",
                    "/drone/src/cmd/revad/revad -c storage-publiclink.toml &",
                    "/drone/src/cmd/revad/revad -c ldap-users.toml",
                ],
            },
            cloneApiTestReposStep(),
            {
                "name": "APIAcceptanceTestsOcisStorage",
                "image": OC_CI_PHP,
                "commands": [
                    "cd /drone/src/tmp/testrunner",
                    "make test-acceptance-from-core-api",
                ],
                "environment": {
                    "TEST_SERVER_URL": "http://revad-services:20080",
                    "OCIS_REVA_DATA_ROOT": "/drone/src/tmp/reva/data/",
                    "DELETE_USER_DATA_CMD": "rm -rf /drone/src/tmp/reva/data/nodes/root/* /drone/src/tmp/reva/data/nodes/*-*-*-* /drone/src/tmp/reva/data/blobs/*",
                    "STORAGE_DRIVER": "OCIS",
                    "SKELETON_DIR": "/drone/src/tmp/testing/data/apiSkeleton",
                    "TEST_WITH_LDAP": "true",
                    "REVA_LDAP_HOSTNAME": "ldap",
                    "TEST_REVA": "true",
                    "SEND_SCENARIO_LINE_REFERENCES": "true",
                    "BEHAT_FILTER_TAGS": "~@provisioning_api-app-required&&~@skipOnOcis-OCIS-Storage&&~@personalSpace&&~@skipOnGraph&&~@carddav&&~@skipOnReva&&~@skipOnRevaMaster",
                    "DIVIDE_INTO_NUM_PARTS": 6,
                    "RUN_PART": 2,
                    "EXPECTED_FAILURES_FILE": "/drone/src/tests/acceptance/expected-failures-on-OCIS-storage.md",
                },
            },
        ],
        "services": [
            ldapService(),
        ],
    }

def checkStarlark():
    return {
        "kind": "pipeline",
        "type": "docker",
        "name": "check-starlark",
        "steps": [
            {
                "name": "format-check-starlark",
                "image": OC_CI_BAZEL_BUILDIFIER,
                "commands": [
                    "buildifier --mode=check .drone.star",
                ],
            },
            {
                "name": "show-diff",
                "image": OC_CI_BAZEL_BUILDIFIER,
                "commands": [
                    "buildifier --mode=fix .drone.star",
                    "git diff",
                ],
                "when": {
                    "status": [
                        "failure",
                    ],
                },
            },
        ],
        "depends_on": [],
        "trigger": {
            "ref": [
                "refs/pull/**",
            ],
        },
    }
