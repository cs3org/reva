def makeStep(target):
    return {
        "name": "build",
        "image": "registry.cern.ch/docker.io/library/golang:1.19",
        "commands": [
            "make %s" % target,
        ],
    }

def cloneOc10TestReposStep():
    return {
        "name": "clone-oC10-test-repos",
        "image": "registry.cern.ch/docker.io/owncloudci/alpine:latest",
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
        "image": "registry.cern.ch/docker.io/osixia/openldap:1.3.0",
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
        "image": "registry.cern.ch/docker.io/webhippie/redis",
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
        release(),
        litmusOcisOldWebdav(),
        litmusOcisNewWebdav(),
        litmusOcisSpacesDav(),
        virtualViews(),
    ] + ocisIntegrationTests(6) + s3ngIntegrationTests(12)

def release():
    return {
        "kind": "pipeline",
        "type": "docker",
        "name": "release",
        "platform": {
            "os": "linux",
            "arch": "amd64",
        },
        "trigger": {
            "event": {
                "include": [
                    "tag",
                ],
            },
        },
        "steps": [
            {
                "name": "create-dist",
                "image": "registry.cern.ch/docker.io/library/golang:1.19",
                "commands": [
                    "go run tools/create-artifacts/main.go -version ${DRONE_TAG} -commit ${DRONE_COMMIT} -goversion `go version | awk '{print $$3}'`",
                ],
            },
            {
                "name": "publish",
                "image": "registry.cern.ch/docker.io/plugins/github-release",
                "settings": {
                    "api_key": {
                        "from_secret": "github_token",
                    },
                    "files": "dist/*",
                    "note": "changelog/NOTE.md",
                },
            },
            {
                "name": "docker-reva-tag",
                "pull": "always",
                "image": "plugins/docker",
                "settings": {
                    "repo": "cs3org/reva",
                    "tags": "${DRONE_TAG}",
                    "dockerfile": "Dockerfile.reva",
                    "username": {
                        "from_secret": "dockerhub_username",
                    },
                    "password": {
                        "from_secret": "dockerhub_password",
                    },
                    "custom_dns": [
                        "128.142.17.5",
                        "128.142.16.5",
                    ],
                },
            },
            {
                "name": "docker-revad-tag",
                "pull": "always",
                "image": "plugins/docker",
                "settings": {
                    "repo": "cs3org/revad",
                    "tags": "${DRONE_TAG}",
                    "dockerfile": "Dockerfile.revad",
                    "username": {
                        "from_secret": "dockerhub_username",
                    },
                    "password": {
                        "from_secret": "dockerhub_password",
                    },
                    "custom_dns": [
                        "128.142.17.5",
                        "128.142.16.5",
                    ],
                },
            },
            {
                "name": "docker-revad-eos-tag",
                "pull": "always",
                "image": "plugins/docker",
                "settings": {
                    "repo": "cs3org/revad",
                    "tags": "${DRONE_TAG}-eos",
                    "dockerfile": "Dockerfile.revad-eos",
                    "username": {
                        "from_secret": "dockerhub_username",
                    },
                    "password": {
                        "from_secret": "dockerhub_password",
                    },
                    "custom_dns": [
                        "128.142.17.5",
                        "128.142.16.5",
                    ],
                },
            },
            {
                "name": "docker-revad-ceph-tag",
                "pull": "always",
                "image": "plugins/docker",
                "settings": {
                    "repo": "cs3org/revad",
                    "tags": "${DRONE_TAG}-ceph",
                    "dockerfile": "Dockerfile.revad-ceph",
                    "username": {
                        "from_secret": "dockerhub_username",
                    },
                    "password": {
                        "from_secret": "dockerhub_password",
                    },
                    "custom_dns": [
                        "128.142.17.5",
                        "128.142.16.5",
                    ],
                },
            },
        ],
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
            "event": {
                "include": [
                    "pull_request",
                    "tag",
                ],
            },
        },
        "steps": [
            makeStep("build-ci"),
            {
                "name": "revad-services",
                "image": "registry.cern.ch/docker.io/library/golang:1.19",
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
                "image": "registry.cern.ch/docker.io/owncloudci/php:7.4",
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
                    "DELETE_USER_DATA_CMD": "rm -rf /drone/src/tmp/reva/data/nodes/root/* /drone/src/tmp/reva/data/nodes/*-*-*-* /drone/src/tmp/reva/data/blobs/*",
                    "STORAGE_DRIVER": "OCIS",
                    "SKELETON_DIR": "/drone/src/tmp/testing/data/apiSkeleton",
                    "TEST_REVA": "true",
                    "REGULAR_USER_PASSWORD": "relativity",
                    "SEND_SCENARIO_LINE_REFERENCES": "true",
                    "BEHAT_SUITE": "apiVirtualViews",
                },
            },
        ],
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
            "event": {
                "include": [
                    "pull_request",
                    "tag",
                ],
            },
        },
        "steps": [
            makeStep("build-ci"),
            {
                "name": "revad-services",
                "image": "registry.cern.ch/docker.io/library/golang:1.19",
                "detach": True,
                "commands": [
                    "cd /drone/src/tests/oc-integration-tests/drone/",
                    "/drone/src/cmd/revad/revad -c frontend.toml &",
                    "/drone/src/cmd/revad/revad -c gateway.toml &",
                    "/drone/src/cmd/revad/revad -c storage-home-ocis.toml &",
                    "/drone/src/cmd/revad/revad -c storage-users-ocis.toml &",
                    "/drone/src/cmd/revad/revad -c users.toml",
                ],
            },
            {
                "name": "sleep-for-revad-start",
                "image": "registry.cern.ch/docker.io/library/golang:1.19",
                "commands": [
                    "sleep 5",
                ],
            },
            {
                "name": "litmus-ocis-old-webdav",
                "image": "registry.cern.ch/docker.io/owncloud/litmus:latest",
                "environment": {
                    "LITMUS_URL": "http://revad-services:20080/remote.php/webdav",
                    "LITMUS_USERNAME": "einstein",
                    "LITMUS_PASSWORD": "relativity",
                    "TESTS": "basic http copymove props",
                },
            },
        ],
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
            "event": {
                "include": [
                    "pull_request",
                    "tag",
                ],
            },
        },
        "steps": [
            makeStep("build-ci"),
            {
                "name": "revad-services",
                "image": "registry.cern.ch/docker.io/library/golang:1.19",
                "detach": True,
                "commands": [
                    "cd /drone/src/tests/oc-integration-tests/drone/",
                    "/drone/src/cmd/revad/revad -c frontend.toml &",
                    "/drone/src/cmd/revad/revad -c gateway.toml &",
                    "/drone/src/cmd/revad/revad -c storage-home-ocis.toml &",
                    "/drone/src/cmd/revad/revad -c storage-users-ocis.toml &",
                    "/drone/src/cmd/revad/revad -c users.toml",
                ],
            },
            {
                "name": "sleep-for-revad-start",
                "image": "registry.cern.ch/docker.io/library/golang:1.19",
                "commands": [
                    "sleep 5",
                ],
            },
            {
                "name": "litmus-ocis-new-webdav",
                "image": "registry.cern.ch/docker.io/owncloud/litmus:latest",
                "environment": {
                    # UUID is einstein user, see https://github.com/owncloud/ocis-accounts/blob/8de0530f31ed5ffb0bbb7f7f3471f87f429cb2ea/pkg/service/v0/service.go#L45
                    "LITMUS_URL": "http://revad-services:20080/remote.php/dav/files/4c510ada-c86b-4815-8820-42cdf82c3d51",
                    "LITMUS_USERNAME": "einstein",
                    "LITMUS_PASSWORD": "relativity",
                    "TESTS": "basic http copymove props",
                },
            },
        ],
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
            "event": {
                "include": [
                    "pull_request",
                    "tag",
                ],
            },
        },
        "steps": [
            makeStep("build-ci"),
            {
                "name": "revad-services",
                "image": "registry.cern.ch/docker.io/library/golang:1.19",
                "detach": True,
                "commands": [
                    "cd /drone/src/tests/oc-integration-tests/drone/",
                    "/drone/src/cmd/revad/revad -c frontend.toml &",
                    "/drone/src/cmd/revad/revad -c gateway.toml &",
                    "/drone/src/cmd/revad/revad -c storage-home-ocis.toml &",
                    "/drone/src/cmd/revad/revad -c storage-users-ocis.toml &",
                    "/drone/src/cmd/revad/revad -c permissions-ocis-ci.toml &",
                    "/drone/src/cmd/revad/revad -c users.toml",
                ],
            },
            {
                "name": "sleep-for-revad-start",
                "image": "registry.cern.ch/docker.io/library/golang:1.19",
                "commands": [
                    "sleep 5",
                ],
            },
            {
                "name": "litmus-owncloud-spaces-dav",
                "image": "registry.cern.ch/docker.io/owncloud/litmus:latest",
                "environment": {
                    "LITMUS_USERNAME": "einstein",
                    "LITMUS_PASSWORD": "relativity",
                    "TESTS": "basic http copymove props",
                },
                "commands": [
                    # The spaceid is randomly generated during the first login so we need this hack to construct the correct url.
                    "curl -s -k -u einstein:relativity -I http://revad-services:20080/remote.php/dav/files/einstein",
                    "export LITMUS_URL=http://revad-services:20080/remote.php/dav/spaces/123e4567-e89b-12d3-a456-426655440000!$(ls /drone/src/tmp/reva/data/spaces/personal/)",
                    "/usr/local/bin/litmus-wrapper",
                ],
            },
        ],
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
                    "event": {
                        "include": [
                            "pull_request",
                            "tag",
                        ],
                    },
                },
                "steps": [
                    makeStep("build-ci"),
                    {
                        "name": "revad-services",
                        "image": "registry.cern.ch/docker.io/library/golang:1.19",
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
                    cloneOc10TestReposStep(),
                    {
                        "name": "oC10APIAcceptanceTestsOcisStorage",
                        "image": "registry.cern.ch/docker.io/owncloudci/php:7.4",
                        "commands": [
                            "cd /drone/src/tmp/testrunner",
                            "composer self-update",
                            "composer --version",
                            "make test-acceptance-api",
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
                            "BEHAT_FILTER_TAGS": "~@notToImplementOnOCIS&&~@toImplementOnOCIS&&~comments-app-required&&~@federation-app-required&&~@notifications-app-required&&~systemtags-app-required&&~@provisioning_api-app-required&&~@preview-extension-required&&~@local_storage&&~@skipOnOcis-OCIS-Storage&&~@skipOnOcis&&~@personalSpace&&~@issue-ocis-3023&&~@skipOnGraph&&~@caldav&&~@carddav&&~@skipOnReva",
                            "DIVIDE_INTO_NUM_PARTS": parallelRuns,
                            "RUN_PART": runPart,
                            "EXPECTED_FAILURES_FILE": "/drone/src/tests/acceptance/expected-failures-on-OCIS-storage.md",
                        },
                    },
                ],
                "services": [
                    ldapService(),
                ],
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
                    "event": {
                        "include": [
                            "pull_request",
                            "tag",
                        ],
                    },
                },
                "steps": [
                    makeStep("build-ci"),
                    {
                        "name": "revad-services",
                        "image": "registry.cern.ch/docker.io/library/golang:1.19",
                        "detach": True,
                        "commands": [
                            "cd /drone/src/tests/oc-integration-tests/drone/",
                            "/drone/src/cmd/revad/revad -c frontend.toml &",
                            "/drone/src/cmd/revad/revad -c gateway.toml &",
                            "/drone/src/cmd/revad/revad -c shares.toml &",
                            "/drone/src/cmd/revad/revad -c storage-home-s3ng.toml &",
                            "/drone/src/cmd/revad/revad -c storage-users-s3ng.toml &",
                            "/drone/src/cmd/revad/revad -c storage-publiclink.toml &",
                            "/drone/src/cmd/revad/revad -c ldap-users.toml",
                        ],
                    },
                    cloneOc10TestReposStep(),
                    {
                        "name": "oC10APIAcceptanceTestsS3ngStorage",
                        "image": "registry.cern.ch/docker.io/owncloudci/php:7.4",
                        "commands": [
                            "cd /drone/src/tmp/testrunner",
                            "composer self-update",
                            "composer --version",
                            "make test-acceptance-api",
                        ],
                        "environment": {
                            "TEST_SERVER_URL": "http://revad-services:20080",
                            "OCIS_REVA_DATA_ROOT": "/drone/src/tmp/reva/data/",
                            "DELETE_USER_DATA_CMD": "rm -rf /drone/src/tmp/reva/data/nodes/root/* /drone/src/tmp/reva/data/nodes/*-*-*-* /drone/src/tmp/reva/data/blobs/*",
                            "STORAGE_DRIVER": "S3NG",
                            "SKELETON_DIR": "/drone/src/tmp/testing/data/apiSkeleton",
                            "TEST_WITH_LDAP": "true",
                            "REVA_LDAP_HOSTNAME": "ldap",
                            "TEST_REVA": "true",
                            "SEND_SCENARIO_LINE_REFERENCES": "true",
                            "BEHAT_FILTER_TAGS": "~@notToImplementOnOCIS&&~@toImplementOnOCIS&&~comments-app-required&&~@federation-app-required&&~@notifications-app-required&&~systemtags-app-required&&~@provisioning_api-app-required&&~@preview-extension-required&&~@local_storage&&~@skipOnOcis-OCIS-Storage&&~@skipOnOcis&&~@personalSpace&&~@issue-ocis-3023&&~&&~@skipOnGraph&&~@caldav&&~@carddav&&~@skipOnReva",
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
                "image": "registry.cern.ch/docker.io/owncloudci/bazel-buildifier:latest",
                "commands": [
                    "buildifier --mode=check .drone.star",
                ],
            },
            {
                "name": "show-diff",
                "image": "registry.cern.ch/docker.io/owncloudci/bazel-buildifier:latest",
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
