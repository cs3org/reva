# images
OC_CI_GOLANG = "owncloudci/golang:1.18"
OC_CI_ALPINE = "owncloudci/alpine:latest"
OSIXIA_OPEN_LDAP = "osixia/openldap:latest"
REDIS = "redis:6-alpine"
OC_CI_PHP = "owncloudci/php:7.4"
OC_LITMUS = "owncloud/litmus:latest"
OC_CS3_API_VALIDATOR = "owncloud/cs3api-validator:0.2.0"
OC_CI_BAZEL_BUILDIFIER = "owncloudci/bazel-buildifier:latest"

# Shared step definitions
def licenseScanStep():
    return {
        "name": "license-scan",
        "image": OC_CI_GOLANG,
        "environment": {
            "FOSSA_API_KEY": {
                "from_secret": "fossa_api_key",
            },
        },
        "detach": True,
        "commands": [
            "wget -qO- https://github.com/fossas/fossa-cli/releases/download/v1.0.11/fossa-cli_1.0.11_linux_amd64.tar.gz | tar xvz -C /go/bin/",
            "/go/bin/fossa analyze",
        ],
    }

def makeStep(target):
    return {
        "name": "build",
        "image": OC_CI_GOLANG,
        "commands": [
            "make %s" % target,
        ],
    }

def cloneOc10TestReposStep():
    return {
        "name": "clone-oC10-test-repos",
        "image": OC_CI_ALPINE,
        "commands": [
            "source /drone/src/.drone.env",
            "git clone -b master --depth=1 https://github.com/owncloud/testing.git /drone/src/tmp/testing",
            "git clone -b $CORE_BRANCH --single-branch --no-tags https://github.com/owncloud/core.git /drone/src/tmp/testrunner",
            "cd /drone/src/tmp/testrunner",
            "git checkout $CORE_COMMITID",
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

def redisService():
    return {
        "name": "redis",
        "image": REDIS,
        "pull": "always",
        "environment": {
            "REDIS_DATABASES": 1,
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
    # In order to run specific parts only, specify the parts as
    # ocisIntegrationTests(6, [1, 4])     - this will only run 1st and 4th parts
    # implemented for: ocisIntegrationTests and s3ngIntegrationTests
    return [
        checkStarlark(),
        checkGoGenerate(),
        coverage(),
        buildOnly(),
        testIntegration(),
        litmusOcisOldWebdav(),
        litmusOcisNewWebdav(),
        litmusOcisSpacesDav(),
        cs3ApiValidatorOcis(),
        cs3ApiValidatorS3NG(),
        # virtual views don't work on edge at the moment
        #virtualViews(),
    ] + ocisIntegrationTests(6) + s3ngIntegrationTests(12)

def coverage():
    return {
        "kind": "pipeline",
        "type": "docker",
        "name": "unit-test-coverage",
        "platform": {
            "os": "linux",
            "arch": "amd64",
        },
        "trigger": {
            "ref": [
                "refs/heads/master",
                "refs/heads/edge",
                "refs/pull/**",
            ],
        },
        "steps": [
            {
                "name": "unit-test",
                "image": OC_CI_GOLANG,
                "commands": [
                    "make test",
                ],
            },
        ],
        "depends_on": [],
    }

def buildOnly():
    return {
        "kind": "pipeline",
        "type": "docker",
        "name": "build-only",
        "platform": {
            "os": "linux",
            "arch": "amd64",
        },
        "trigger": {
            "ref": [
                "refs/heads/master",
                "refs/heads/edge",
                "refs/pull/**",
            ],
        },
        "steps": [
            makeStep("dist"),
        ],
        "depends_on": ["unit-test-coverage"],
    }

def testIntegration():
    return {
        "kind": "pipeline",
        "type": "docker",
        "name": "test-integration",
        "platform": {
            "os": "linux",
            "arch": "amd64",
        },
        "trigger": {
            "ref": [
                "refs/heads/master",
                "refs/heads/edge",
                "refs/pull/**",
            ],
        },
        "steps": [
            {
                "name": "test",
                "image": OC_CI_GOLANG,
                "commands": [
                    "make test-integration",
                ],
                "environment": {
                    "REDIS_ADDRESS": "redis:6379",
                },
            },
        ],
        "services": [
            redisService(),
        ],
        "depends_on": ["unit-test-coverage"],
    }

def virtualViews():
    return {
        "kind": "pipeline",
        "type": "docker",
        "name": "virtual-views",
        "platform": {
            "os": "linux",
            "arch": "amd64",
        },
        "trigger": {
            "ref": [
                "refs/heads/master",
                "refs/heads/edge",
                "refs/pull/**",
            ],
        },
        "steps": [
            makeStep("build-ci"),
            {
                "name": "revad-services",
                "image": OC_CI_GOLANG,
                "detach": True,
                "commands": [
                    "cd /drone/src/tests/oc-integration-tests/drone/",
                    "/drone/src/cmd/revad/revad -c frontend-global.toml &",
                    "/drone/src/cmd/revad/revad -c gateway.toml &",
                    "/drone/src/cmd/revad/revad -c storage-home-ocis.toml &",
                    "/drone/src/cmd/revad/revad -c storage-local-1.toml &",
                    "/drone/src/cmd/revad/revad -c storage-local-2.toml &",
                    "/drone/src/cmd/revad/revad -c users.toml",
                ],
            },
            cloneOc10TestReposStep(),
            {
                "name": "oC10APIAcceptanceTestsOcisStorage",
                "image": OC_CI_PHP,
                "commands": [
                    "cd /drone/src",
                    "composer self-update",
                    "composer --version",
                    "make test-acceptance-api",
                ],
                "environment": {
                    "PATH_TO_CORE": "/drone/src/tmp/testrunner",
                    "TEST_SERVER_URL": "http://revad-services:20180",
                    "OCIS_REVA_DATA_ROOT": "/drone/src/tmp/reva/data/",
                    "DELETE_USER_DATA_CMD": "rm -rf /drone/src/tmp/reva/data/spaces/* /drone/src/tmp/reva/data/blobs/* /drone/src/tmp/reva/data/indexes",
                    "STORAGE_DRIVER": "OCIS",
                    "SKELETON_DIR": "/drone/src/tmp/testing/data/apiSkeleton",
                    "TEST_REVA": "true",
                    "REGULAR_USER_PASSWORD": "relativity",
                    "SEND_SCENARIO_LINE_REFERENCES": "true",
                    "BEHAT_SUITE": "apiVirtualViews",
                },
            },
        ],
        "depends_on": ["unit-test-coverage"],
    }

def litmusOcisOldWebdav():
    return {
        "kind": "pipeline",
        "type": "docker",
        "name": "litmus-ocis-old-webdav",
        "platform": {
            "os": "linux",
            "arch": "amd64",
        },
        "trigger": {
            "ref": [
                "refs/heads/master",
                "refs/heads/edge",
                "refs/pull/**",
            ],
        },
        "steps": [
            makeStep("build-ci"),
            {
                "name": "revad-services",
                "image": OC_CI_GOLANG,
                "detach": True,
                "commands": [
                    "cd /drone/src/tests/oc-integration-tests/drone/",
                    "/drone/src/cmd/revad/revad -c frontend.toml &",
                    "/drone/src/cmd/revad/revad -c gateway.toml &",
                    "/drone/src/cmd/revad/revad -c storage-users-ocis.toml &",
                    "/drone/src/cmd/revad/revad -c storage-shares.toml &",
                    "/drone/src/cmd/revad/revad -c storage-publiclink.toml &",
                    "/drone/src/cmd/revad/revad -c shares.toml &",
                    "/drone/src/cmd/revad/revad -c permissions-ocis-ci.toml &",
                    "/drone/src/cmd/revad/revad -c users.toml",
                ],
            },
            {
                "name": "sleep-for-revad-start",
                "image": OC_CI_GOLANG,
                "commands": [
                    "sleep 5",
                ],
            },
            {
                "name": "litmus-ocis-old-webdav",
                "image": OC_LITMUS,
                "environment": {
                    "LITMUS_URL": "http://revad-services:20080/remote.php/webdav",
                    "LITMUS_USERNAME": "einstein",
                    "LITMUS_PASSWORD": "relativity",
                    "TESTS": "basic http copymove props",
                },
            },
        ],
        "depends_on": ["unit-test-coverage"],
    }

def litmusOcisNewWebdav():
    return {
        "kind": "pipeline",
        "type": "docker",
        "name": "litmus-ocis-new-webdav",
        "platform": {
            "os": "linux",
            "arch": "amd64",
        },
        "trigger": {
            "ref": [
                "refs/heads/master",
                "refs/heads/edge",
                "refs/pull/**",
            ],
        },
        "steps": [
            makeStep("build-ci"),
            {
                "name": "revad-services",
                "image": OC_CI_GOLANG,
                "detach": True,
                "commands": [
                    "cd /drone/src/tests/oc-integration-tests/drone/",
                    "/drone/src/cmd/revad/revad -c frontend.toml &",
                    "/drone/src/cmd/revad/revad -c gateway.toml &",
                    "/drone/src/cmd/revad/revad -c storage-users-ocis.toml &",
                    "/drone/src/cmd/revad/revad -c storage-shares.toml &",
                    "/drone/src/cmd/revad/revad -c storage-publiclink.toml &",
                    "/drone/src/cmd/revad/revad -c shares.toml &",
                    "/drone/src/cmd/revad/revad -c permissions-ocis-ci.toml &",
                    "/drone/src/cmd/revad/revad -c users.toml",
                ],
            },
            {
                "name": "sleep-for-revad-start",
                "image": OC_CI_GOLANG,
                "commands": [
                    "sleep 5",
                ],
            },
            {
                "name": "litmus-ocis-new-webdav",
                "image": OC_LITMUS,
                "environment": {
                    # UUID is einstein user, see https://github.com/owncloud/ocis-accounts/blob/8de0530f31ed5ffb0bbb7f7f3471f87f429cb2ea/pkg/service/v0/service.go#L45
                    "LITMUS_URL": "http://revad-services:20080/remote.php/dav/files/4c510ada-c86b-4815-8820-42cdf82c3d51",
                    "LITMUS_USERNAME": "einstein",
                    "LITMUS_PASSWORD": "relativity",
                    "TESTS": "basic http copymove props",
                },
            },
        ],
        "depends_on": ["unit-test-coverage"],
    }

def litmusOcisSpacesDav():
    return {
        "kind": "pipeline",
        "type": "docker",
        "name": "litmus-owncloud-spaces-dav",
        "platform": {
            "os": "linux",
            "arch": "amd64",
        },
        "trigger": {
            "ref": [
                "refs/heads/master",
                "refs/heads/edge",
                "refs/pull/**",
            ],
        },
        "steps": [
            makeStep("build-ci"),
            {
                "name": "revad-services",
                "image": OC_CI_GOLANG,
                "detach": True,
                "commands": [
                    "cd /drone/src/tests/oc-integration-tests/drone/",
                    "/drone/src/cmd/revad/revad -c frontend.toml &",
                    "/drone/src/cmd/revad/revad -c gateway.toml &",
                    "/drone/src/cmd/revad/revad -c storage-users-ocis.toml &",
                    "/drone/src/cmd/revad/revad -c storage-shares.toml &",
                    "/drone/src/cmd/revad/revad -c storage-publiclink.toml &",
                    "/drone/src/cmd/revad/revad -c shares.toml &",
                    "/drone/src/cmd/revad/revad -c permissions-ocis-ci.toml &",
                    "/drone/src/cmd/revad/revad -c users.toml",
                ],
            },
            {
                "name": "sleep-for-revad-start",
                "image": OC_CI_GOLANG,
                "commands": [
                    "sleep 5",
                ],
            },
            {
                "name": "litmus-owncloud-spaces-dav",
                "image": OC_LITMUS,
                "environment": {
                    "LITMUS_USERNAME": "einstein",
                    "LITMUS_PASSWORD": "relativity",
                    "TESTS": "basic http copymove props",
                },
                "commands": [
                    # The spaceid is randomly generated during the first login so we need this hack to construct the correct url.
                    "curl -s -k -u einstein:relativity -I http://revad-services:20080/remote.php/dav/files/einstein",
                    "export LITMUS_URL=http://revad-services:20080/remote.php/dav/spaces/$(ls /drone/src/tmp/reva/data/indexes/by-type/personal/)",
                    "/usr/local/bin/litmus-wrapper",
                ],
            },
        ],
        "depends_on": ["unit-test-coverage"],
    }

def cs3ApiValidatorOcis():
    return {
        "kind": "pipeline",
        "type": "docker",
        "name": "cs3api-validator-ocis",
        "platform": {
            "os": "linux",
            "arch": "amd64",
        },
        "trigger": {
            "ref": [
                "refs/heads/master",
                "refs/heads/edge",
                "refs/pull/**",
            ],
        },
        "steps": [
            makeStep("build-ci"),
            {
                "name": "revad-services",
                "image": OC_CI_GOLANG,
                "detach": True,
                "commands": [
                    "cd /drone/src/tests/oc-integration-tests/drone/",
                    "/drone/src/cmd/revad/revad -c frontend.toml &",
                    "/drone/src/cmd/revad/revad -c gateway.toml &",
                    "/drone/src/cmd/revad/revad -c storage-users-ocis.toml &",
                    "/drone/src/cmd/revad/revad -c storage-shares.toml &",
                    "/drone/src/cmd/revad/revad -c storage-publiclink.toml &",
                    "/drone/src/cmd/revad/revad -c shares.toml &",
                    "/drone/src/cmd/revad/revad -c permissions-ocis-ci.toml &",
                    "/drone/src/cmd/revad/revad -c users.toml",
                ],
            },
            {
                "name": "sleep-for-revad-start",
                "image": OC_CI_GOLANG,
                "commands": [
                    "sleep 5",
                ],
            },
            {
                "name": "cs3api-validator-ocis",
                "image": OC_CS3_API_VALIDATOR,
                "commands": [
                    "/usr/bin/cs3api-validator /var/lib/cs3api-validator --endpoint=revad-services:19000",
                ],
            },
        ],
        "depends_on": ["unit-test-coverage"],
    }

def cs3ApiValidatorS3NG():
    return {
        "kind": "pipeline",
        "type": "docker",
        "name": "cs3api-validator-S3NG",
        "platform": {
            "os": "linux",
            "arch": "amd64",
        },
        "trigger": {
            "ref": [
                "refs/heads/master",
                "refs/heads/edge",
                "refs/pull/**",
            ],
        },
        "steps": [
            makeStep("build-ci"),
            {
                "name": "revad-services",
                "image": OC_CI_GOLANG,
                "detach": True,
                "commands": [
                    "cd /drone/src/tests/oc-integration-tests/drone/",
                    "/drone/src/cmd/revad/revad -c frontend.toml &",
                    "/drone/src/cmd/revad/revad -c gateway.toml &",
                    "/drone/src/cmd/revad/revad -c storage-users-s3ng.toml &",
                    "/drone/src/cmd/revad/revad -c storage-shares.toml &",
                    "/drone/src/cmd/revad/revad -c storage-publiclink.toml &",
                    "/drone/src/cmd/revad/revad -c shares.toml &",
                    "/drone/src/cmd/revad/revad -c permissions-ocis-ci.toml &",
                    "/drone/src/cmd/revad/revad -c users.toml",
                ],
            },
            {
                "name": "sleep-for-revad-start",
                "image": OC_CI_GOLANG,
                "commands": [
                    "sleep 5",
                ],
            },
            {
                "name": "cs3api-validator-S3NG",
                "image": OC_CS3_API_VALIDATOR,
                "commands": [
                    "/usr/bin/cs3api-validator /var/lib/cs3api-validator --endpoint=revad-services:19000",
                ],
            },
        ],
        "services": [
            cephService(),
        ],
        "depends_on": ["unit-test-coverage"],
    }

def ocisIntegrationTests(parallelRuns, skipExceptParts = []):
    pipelines = []
    debugPartsEnabled = (len(skipExceptParts) != 0)
    for runPart in range(1, parallelRuns + 1):
        if debugPartsEnabled and runPart not in skipExceptParts:
            continue

        pipelines.append(
            {
                "kind": "pipeline",
                "type": "docker",
                "name": "ocis-integration-tests-%s" % runPart,
                "platform": {
                    "os": "linux",
                    "arch": "amd64",
                },
                "trigger": {
                    "ref": [
                        "refs/heads/master",
                        "refs/heads/edge",
                        "refs/pull/**",
                    ],
                },
                "steps": [
                    makeStep("build-ci"),
                    {
                        "name": "revad-services",
                        "image": OC_CI_GOLANG,
                        "detach": True,
                        "commands": [
                            "cd /drone/src/tests/oc-integration-tests/drone/",
                            "/drone/src/cmd/revad/revad -c frontend.toml &",
                            "/drone/src/cmd/revad/revad -c gateway.toml &",
                            "/drone/src/cmd/revad/revad -c shares.toml &",
                            "/drone/src/cmd/revad/revad -c storage-shares.toml &",
                            "/drone/src/cmd/revad/revad -c machine-auth.toml &",
                            "/drone/src/cmd/revad/revad -c storage-users-ocis.toml &",
                            "/drone/src/cmd/revad/revad -c storage-publiclink.toml &",
                            "/drone/src/cmd/revad/revad -c permissions-ocis-ci.toml &",
                            "/drone/src/cmd/revad/revad -c ldap-users.toml",
                        ],
                    },
                    cloneOc10TestReposStep(),
                    {
                        "name": "oC10APIAcceptanceTestsOcisStorage",
                        "image": OC_CI_PHP,
                        "commands": [
                            "cd /drone/src/tmp/testrunner",
                            "composer self-update",
                            "composer --version",
                            "make test-acceptance-api",
                        ],
                        "environment": {
                            "TEST_SERVER_URL": "http://revad-services:20080",
                            "OCIS_REVA_DATA_ROOT": "/drone/src/tmp/reva/data/",
                            "DELETE_USER_DATA_CMD": "rm -rf /drone/src/tmp/reva/data/spaces/* /drone/src/tmp/reva/data/blobs/* /drone/src/tmp/reva/data/indexes/by-type/*",
                            "STORAGE_DRIVER": "OCIS",
                            "SKELETON_DIR": "/drone/src/tmp/testing/data/apiSkeleton",
                            "TEST_WITH_LDAP": "true",
                            "REVA_LDAP_HOSTNAME": "ldap",
                            "TEST_REVA": "true",
                            "SEND_SCENARIO_LINE_REFERENCES": "true",
                            "BEHAT_FILTER_TAGS": "~@notToImplementOnOCIS&&~@toImplementOnOCIS&&~comments-app-required&&~@federation-app-required&&~@notifications-app-required&&~systemtags-app-required&&~@provisioning_api-app-required&&~@preview-extension-required&&~@local_storage&&~@skipOnOcis-OCIS-Storage&&~@skipOnOcis&&~@skipOnGraph&&~@caldav&&~@carddav&&~@skipOnReva",
                            "DIVIDE_INTO_NUM_PARTS": parallelRuns,
                            "RUN_PART": runPart,
                            "EXPECTED_FAILURES_FILE": "/drone/src/tests/acceptance/expected-failures-on-OCIS-storage.md",
                        },
                    },
                ],
                "services": [
                    ldapService(),
                ],
                "depends_on": ["unit-test-coverage"],
            },
        )

    return pipelines

def s3ngIntegrationTests(parallelRuns, skipExceptParts = []):
    pipelines = []
    debugPartsEnabled = (len(skipExceptParts) != 0)
    for runPart in range(1, parallelRuns + 1):
        if debugPartsEnabled and runPart not in skipExceptParts:
            continue

        pipelines.append(
            {
                "kind": "pipeline",
                "type": "docker",
                "name": "s3ng-integration-tests-%s" % runPart,
                "platform": {
                    "os": "linux",
                    "arch": "amd64",
                },
                "trigger": {
                    "ref": [
                        "refs/heads/master",
                        "refs/heads/edge",
                        "refs/pull/**",
                    ],
                },
                "steps": [
                    makeStep("build-ci"),
                    {
                        "name": "revad-services",
                        "image": OC_CI_GOLANG,
                        "detach": True,
                        "commands": [
                            "cd /drone/src/tests/oc-integration-tests/drone/",
                            "/drone/src/cmd/revad/revad -c frontend.toml &",
                            "/drone/src/cmd/revad/revad -c gateway.toml &",
                            "/drone/src/cmd/revad/revad -c shares.toml &",
                            "/drone/src/cmd/revad/revad -c storage-users-s3ng.toml &",
                            "/drone/src/cmd/revad/revad -c storage-publiclink.toml &",
                            "/drone/src/cmd/revad/revad -c storage-shares.toml &",
                            "/drone/src/cmd/revad/revad -c ldap-users.toml &",
                            "/drone/src/cmd/revad/revad -c permissions-ocis-ci.toml &",
                            "/drone/src/cmd/revad/revad -c machine-auth.toml",
                        ],
                    },
                    cloneOc10TestReposStep(),
                    {
                        "name": "oC10APIAcceptanceTestsS3ngStorage",
                        "image": OC_CI_PHP,
                        "commands": [
                            "cd /drone/src/tmp/testrunner",
                            "composer self-update",
                            "composer --version",
                            "make test-acceptance-api",
                        ],
                        "environment": {
                            "TEST_SERVER_URL": "http://revad-services:20080",
                            "OCIS_REVA_DATA_ROOT": "/drone/src/tmp/reva/data/",
                            "DELETE_USER_DATA_CMD": "rm -rf /drone/src/tmp/reva/data/spaces/* /drone/src/tmp/reva/data/blobs/* /drone/src/tmp/reva/data/indexes/by-type/*",
                            "STORAGE_DRIVER": "S3NG",
                            "SKELETON_DIR": "/drone/src/tmp/testing/data/apiSkeleton",
                            "TEST_WITH_LDAP": "true",
                            "REVA_LDAP_HOSTNAME": "ldap",
                            "TEST_REVA": "true",
                            "SEND_SCENARIO_LINE_REFERENCES": "true",
                            "BEHAT_FILTER_TAGS": "~@notToImplementOnOCIS&&~@toImplementOnOCIS&&~comments-app-required&&~@federation-app-required&&~@notifications-app-required&&~systemtags-app-required&&~@provisioning_api-app-required&&~@preview-extension-required&&~@local_storage&&~@skipOnOcis-OCIS-Storage&&~@skipOnOcis&&~@skipOnGraph&&~@caldav&&~@carddav&&~@skipOnReva",
                            "DIVIDE_INTO_NUM_PARTS": parallelRuns,
                            "RUN_PART": runPart,
                            "EXPECTED_FAILURES_FILE": "/drone/src/tests/acceptance/expected-failures-on-S3NG-storage.md",
                        },
                    },
                ],
                "services": [
                    ldapService(),
                    cephService(),
                ],
                "depends_on": ["unit-test-coverage"],
            },
        )

    return pipelines

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

def checkGoGenerate():
    return {
        "kind": "pipeline",
        "type": "docker",
        "name": "check-go-generate",
        "steps": [
            {
                "name": "check-go-generate",
                "image": OC_CI_GOLANG,
                "commands": [
                    "make go-generate",
                    "git diff --exit-code",
                ],
            },
        ],
        "depends_on": [],
        "trigger": {
            "ref": [
                "refs/heads/master",
                "refs/heads/edge",
                "refs/pull/**",
            ],
        },
    }
