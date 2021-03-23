# Shared step definitions
def licenseScanStep():
  return {
    "name": "license-scan",
    "image": "registry.cern.ch/docker.io/library/golang:1.16",
    "environment": {
      "FOSSA_API_KEY": {
        "from_secret": "fossa_api_key",
      },
    },
    "detach": True,
    "commands": [
      "wget -qO- https://github.com/fossas/fossa-cli/releases/download/v1.0.11/fossa-cli_1.0.11_linux_amd64.tar.gz | tar xvz -C /go/bin/",
      "/go/bin/fossa analyze",
    ]
  }

def makeStep(target):
  return {
    "name": "build",
    "image": "registry.cern.ch/docker.io/library/golang:1.16",
    "commands": [
      "make %s" % target,
    ]
  }

def lintStep():
  return {
    "name": "lint",
    "image": "registry.cern.ch/docker.io/golangci/golangci-lint:v1.38",
    "commands": [
      "golangci-lint run --timeout 2m0s",
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
    }
  }

# Pipeline definitions
def main(ctx):
  return [
    buildAndPublishDocker(),
    buildOnly(),
    testIntegration(),
    release(),
    litmusOcisOldWebdav(),
    litmusOcisNewWebdav(),
    localIntegrationTestsOwncloud(),
    localIntegrationTestsOcis(),
  ] + ocisIntegrationTests(6) + owncloudIntegrationTests(6) + s3ngIntegrationTests(12)


def buildAndPublishDocker():
  return {
    "kind": "pipeline",
    "type": "docker",
    "name": "build-and-publish-docker",
    "platform": {
      "os": "linux",
      "arch": "amd64",
    },
    "trigger": {
      "branch":[
        "master"
      ],
      "event": {
        "exclude": [
          "pull_request",
          "tag",
          "promote",
          "rollback",
        ],
      },
    },
    "steps": [
      {
        "name": "store-dev-release",
        "image": "registry.cern.ch/docker.io/library/golang:1.16",
        "environment": {
          "USERNAME": {
            "from_secret": "cbox_username",
          },
          "PASSWORD": {
            "from_secret": "cbox_password",
          },
        },
        "detach": True,
        "commands": [
          "TZ=Europe/Berlin go run tools/create-artifacts/main.go -dev -commit ${DRONE_COMMIT} -goversion `go version | awk '{print $$3}'`",
          "curl --fail -X MKCOL 'https://cernbox.cern.ch/cernbox/desktop/remote.php/webdav/eos/project/r/reva/www/daily/' -k -u $${USERNAME}:$${PASSWORD}",
          "curl --fail -X MKCOL 'https://cernbox.cern.ch/cernbox/desktop/remote.php/webdav/eos/project/r/reva/www/daily/`date '+%Y-%m-%d'`' -k -u $${USERNAME}:$${PASSWORD}",
          "curl --fail -X MKCOL 'https://cernbox.cern.ch/cernbox/desktop/remote.php/webdav/eos/project/r/reva/www/daily/`date '+%Y-%m-%d'`/${DRONE_COMMIT}' -k -u $${USERNAME}:$${PASSWORD}",
          "for i in $(ls /drone/src/dist);do curl --fail -X PUT -u $${USERNAME}:$${PASSWORD} https://cernbox.cern.ch/cernbox/desktop/remote.php/webdav/eos/project/r/reva/www/daily/`date '+%Y-%m-%d'`/${DRONE_COMMIT}/$${i} --data-binary @./dist/$${i} ; done",
        ],
      },
      licenseScanStep(),
      makeStep("ci"),
      lintStep(),
      {
        "name": "license-check",
        "image": "registry.cern.ch/docker.io/library/golang:1.16",
        "failure": "ignore",
        "environment":{
          "FOSSA_API_KEY":{
            "from_secret": "fossa_api_key",
          },
        },
        "commands": [
          "wget -qO- https://github.com/fossas/fossa-cli/releases/download/v1.0.11/fossa-cli_1.0.11_linux_amd64.tar.gz | tar xvz -C /go/bin/",
          "/go/bin/fossa test --timeout 900",
        ]
      },
      {
        "name": "publish-docker-reva-latest",
        "pull": "always",
        "image": "plugins/docker",
        "settings": {
          "repo": "cs3org/reva",
          "tags": "latest",
          "dockerfile": "Dockerfile.revad",
          "username":{
            "from_secret": "dockerhub_username",
          },
          "password": {
            "from_secret": "dockerhub_password",
          },
          "custom_dns":[
            "128.142.17.5",
            "128.142.16.5",
          ],
        }
      },
      {
        "name": "publish-docker-revad-latest",
        "pull": "always",
        "image": "plugins/docker",
        "settings":{
          "repo": "cs3org/revad",
          "tags": "latest",
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
        }
      },
      {
        "name": "publish-docker-revad-eos-latest",
        "pull": "always",
        "image": "plugins/docker",
        "settings": {
          "repo": "cs3org/revad",
          "tags": "latest-eos",
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
          ]
        }
      }
    ]
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
    "trigger":{
      "event": {
        "include": [
          "pull_request",
        ],
      },
    },
    "steps": [
      licenseScanStep(),
      makeStep("ci"),
      lintStep(),
      {
        "name": "changelog",
        "image": "registry.cern.ch/docker.io/library/golang:1.16",
        "commands": [
          "make release-deps && /go/bin/calens > /dev/null",
          "make check-changelog-drone PR=$DRONE_PULL_REQUEST",
        ],
        "environment": {
          "GITHUB_API_TOKEN": {
            "from_secret": "github_api_token",
          },
        },
      },
      {
        "name": "license-check",
        "image": "registry.cern.ch/docker.io/library/golang:1.16",
        "failure": "ignore",
        "environment": {
          "FOSSA_API_KEY": {
            "from_secret": "fossa_api_key",
          },
        },
        "commands": [
          "wget -qO- https://github.com/fossas/fossa-cli/releases/download/v1.0.11/fossa-cli_1.0.11_linux_amd64.tar.gz | tar xvz -C /go/bin/",
          "/go/bin/fossa test --timeout 900",
        ],
      },
    ]
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
      "event": {
        "include": [
          "pull_request",
        ],
      },
    },
    "steps": [
      {
        "name": "test",
        "image": "registry.cern.ch/docker.io/library/golang:1.16",
        "commands": [
          "make test-integration",
        ],
        "environment": {
          "REDIS_ADDRESS": "redis:6379",
        },
      }
    ],
    "services": [
      redisService(),
    ],
  }

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
      licenseScanStep(),
      makeStep("ci"),
      lintStep(),
      {
        "name": "create-dist",
        "image": "registry.cern.ch/docker.io/library/golang:1.16",
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
          "custom_dns":[
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
    ],
  }

def litmusOcisOldWebdav():
  return {
    "kind": "pipeline",
    "type": "docker",
    "name": "litmus-owncloud-old-webdav",
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
        "image": "registry.cern.ch/docker.io/library/golang:1.16",
        "detach": True,
        "commands": [
          "cd /drone/src/tests/oc-integration-tests/drone/",
          "/drone/src/cmd/revad/revad -c frontend.toml &",
          "/drone/src/cmd/revad/revad -c gateway.toml &",
          "/drone/src/cmd/revad/revad -c storage-home-owncloud.toml &",
          "/drone/src/cmd/revad/revad -c storage-oc-owncloud.toml &",
          "/drone/src/cmd/revad/revad -c users.toml",
        ],
      },
      {
        "name": "sleep-for-revad-start",
        "image": "registry.cern.ch/docker.io/library/golang:1.16",
        "commands": [
          "sleep 5",
        ],
      },
      {
        "name": "litmus-owncloud-old-webdav",
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
    "name": "litmus-owncloud-new-webdav",
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
        "image": "registry.cern.ch/docker.io/library/golang:1.16",
        "detach": True,
        "commands": [
          "cd /drone/src/tests/oc-integration-tests/drone/",
          "/drone/src/cmd/revad/revad -c frontend.toml &",
          "/drone/src/cmd/revad/revad -c gateway.toml &",
          "/drone/src/cmd/revad/revad -c storage-home-owncloud.toml &",
          "/drone/src/cmd/revad/revad -c storage-oc-owncloud.toml &",
          "/drone/src/cmd/revad/revad -c users.toml",
        ]
      },
      {
        "name": "sleep-for-revad-start",
        "image": "registry.cern.ch/docker.io/library/golang:1.16",
        "commands":[
          "sleep 5",
        ],
      },
      {
        "name": "litmus-owncloud-new-webdav",
        "image": "registry.cern.ch/docker.io/owncloud/litmus:latest",
        "environment": {
          # UUID is einstein user, see https://github.com/owncloud/ocis-accounts/blob/8de0530f31ed5ffb0bbb7f7f3471f87f429cb2ea/pkg/service/v0/service.go#L45
          "LITMUS_URL": "http://revad-services:20080/remote.php/dav/files/4c510ada-c86b-4815-8820-42cdf82c3d51",
          "LITMUS_USERNAME": "einstein",
          "LITMUS_PASSWORD": "relativity",
          "TESTS": "basic http copymove props",
        }
      },
    ],
  }

def localIntegrationTestsOwncloud():
  return {
    "kind": "pipeline",
    "type": "docker",
    "name": "local-integration-tests-owncloud",
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
        "image": "registry.cern.ch/docker.io/library/golang:1.16",
        "detach": True,
        "commands": [
          "cd /drone/src/tests/oc-integration-tests/drone/",
          "/drone/src/cmd/revad/revad -c frontend.toml &",
          "/drone/src/cmd/revad/revad -c gateway.toml &",
          "/drone/src/cmd/revad/revad -c shares.toml &",
          "/drone/src/cmd/revad/revad -c storage-home-owncloud.toml &",
          "/drone/src/cmd/revad/revad -c storage-oc-owncloud.toml &",
          "/drone/src/cmd/revad/revad -c storage-publiclink-owncloud.toml &",
          "/drone/src/cmd/revad/revad -c ldap-users.toml",
        ],
      },
      cloneOc10TestReposStep(),
      {
        "name": "localAPIAcceptanceTestsOwncloudStorage",
        "image": "registry.cern.ch/docker.io/owncloudci/php:7.4",
        "commands": [
          "make test-acceptance-api",
        ],
        "environment": {
          "TEST_SERVER_URL": "http://revad-services:20080",
          "OCIS_REVA_DATA_ROOT": "/drone/src/tmp/reva/data/",
          "STORAGE_DRIVER": "OWNCLOUD",
          "SKELETON_DIR": "/drone/src/tmp/testing/data/apiSkeleton",
          "TEST_WITH_LDAP": "true",
          "REVA_LDAP_HOSTNAME": "ldap",
          "TEST_REVA": "true",
          "BEHAT_FILTER_TAGS": "~@skipOnOcis-OC-Storage",
          "PATH_TO_CORE": "/drone/src/tmp/testrunner",
        }
      }
    ],
    "services": [
      ldapService(),
      redisService(),
    ],
  }

def localIntegrationTestsOcis():
  return {
    "kind": "pipeline",
    "type": "docker",
    "name": "local-integration-tests-ocis",
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
        "image": "registry.cern.ch/docker.io/library/golang:1.16",
        "detach": True,
        "commands": [
          "cd /drone/src/tests/oc-integration-tests/drone/",
          "/drone/src/cmd/revad/revad -c frontend.toml &",
          "/drone/src/cmd/revad/revad -c gateway.toml &",
          "/drone/src/cmd/revad/revad -c shares.toml &",
          "/drone/src/cmd/revad/revad -c storage-home-ocis.toml &",
          "/drone/src/cmd/revad/revad -c storage-oc-ocis.toml &",
          "/drone/src/cmd/revad/revad -c storage-publiclink-ocis.toml &",
          "/drone/src/cmd/revad/revad -c ldap-users.toml",
        ],
      },
      cloneOc10TestReposStep(),
      {
        "name": "localAPIAcceptanceTestsOcisStorage",
        "image": "registry.cern.ch/docker.io/owncloudci/php:7.4",
        "commands": [
          "make test-acceptance-api",
        ],
        "environment": {
          "TEST_SERVER_URL": "http://revad-services:20080",
          "OCIS_REVA_DATA_ROOT": "/drone/src/tmp/reva/data/",
          "DELETE_USER_DATA_CMD": "rm -rf /drone/src/tmp/reva/data/nodes/root/* /drone/src/tmp/reva/data/nodes/*-*-*-*",
          "STORAGE_DRIVER": "OCIS",
          "SKELETON_DIR": "/drone/src/tmp/testing/data/apiSkeleton",
          "TEST_WITH_LDAP": "true",
          "REVA_LDAP_HOSTNAME": "ldap",
          "TEST_REVA": "true",
          "BEHAT_FILTER_TAGS": "~@skipOnOcis-OCIS-Storage",
          "PATH_TO_CORE": "/drone/src/tmp/testrunner",
        }
      },
    ],
    "services": [
      ldapService(),
    ],
  }

def ocisIntegrationTests(parallelRuns):
  pipelines = []
  for runPart in range(1, parallelRuns + 1):
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
            "image": "registry.cern.ch/docker.io/library/golang:1.16",
            "detach": True,
            "commands": [
              "cd /drone/src/tests/oc-integration-tests/drone/",
              "/drone/src/cmd/revad/revad -c frontend.toml &",
              "/drone/src/cmd/revad/revad -c gateway.toml &",
              "/drone/src/cmd/revad/revad -c shares.toml &",
              "/drone/src/cmd/revad/revad -c storage-home-ocis.toml &",
              "/drone/src/cmd/revad/revad -c storage-oc-ocis.toml &",
              "/drone/src/cmd/revad/revad -c storage-publiclink-ocis.toml &",
              "/drone/src/cmd/revad/revad -c ldap-users.toml",
            ],
          },
          cloneOc10TestReposStep(),
          {
            "name": "oC10APIAcceptanceTestsOcisStorage",
            "image": "registry.cern.ch/docker.io/owncloudci/php:7.4",
            "commands": [
              "cd /drone/src/tmp/testrunner",
              "make test-acceptance-api",
            ],
            "environment": {
              "TEST_SERVER_URL": "http://revad-services:20080",
              "OCIS_REVA_DATA_ROOT": "/drone/src/tmp/reva/data/",
              "DELETE_USER_DATA_CMD": "rm -rf /drone/src/tmp/reva/data/nodes/root/* /drone/src/tmp/reva/data/nodes/*-*-*-*",
              "STORAGE_DRIVER": "OCIS",
              "SKELETON_DIR": "/drone/src/tmp/testing/data/apiSkeleton",
              "TEST_WITH_LDAP": "true",
              "REVA_LDAP_HOSTNAME": "ldap",
              "TEST_REVA": "true",
              "BEHAT_FILTER_TAGS": "~@notToImplementOnOCIS&&~@toImplementOnOCIS&&~comments-app-required&&~@federation-app-required&&~@notifications-app-required&&~systemtags-app-required&&~@provisioning_api-app-required&&~@preview-extension-required&&~@local_storage&&~@skipOnOcis-OCIS-Storage",
              "DIVIDE_INTO_NUM_PARTS": parallelRuns,
              "RUN_PART": runPart,
              "EXPECTED_FAILURES_FILE": "/drone/src/tests/acceptance/expected-failures-on-OCIS-storage.md",
            },
          },
        ],
        "services": [
          ldapService(),
        ],
      }
    )

  return pipelines

def owncloudIntegrationTests(parallelRuns):
  pipelines = []
  for runPart in range(1, parallelRuns + 1):
    pipelines.append(
      {
        "kind": "pipeline",
        "type": "docker",
        "name": "owncloud-integration-tests-%s" % runPart,
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
            "image": "registry.cern.ch/docker.io/library/golang:1.16",
            "detach": True,
            "commands": [
              "cd /drone/src/tests/oc-integration-tests/drone/",
              "/drone/src/cmd/revad/revad -c frontend.toml &",
              "/drone/src/cmd/revad/revad -c gateway.toml &",
              "/drone/src/cmd/revad/revad -c shares.toml &",
              "/drone/src/cmd/revad/revad -c storage-home-owncloud.toml &",
              "/drone/src/cmd/revad/revad -c storage-oc-owncloud.toml &",
              "/drone/src/cmd/revad/revad -c storage-publiclink-owncloud.toml &",
              "/drone/src/cmd/revad/revad -c ldap-users.toml",
            ],
          },
          cloneOc10TestReposStep(),
          {
            "name": "oC10APIAcceptanceTestsOwncloudStorage",
            "image": "registry.cern.ch/docker.io/owncloudci/php:7.4",
            "commands": [
              "cd /drone/src/tmp/testrunner",
              "make test-acceptance-api",
            ],
            "environment": {
              "TEST_SERVER_URL": "http://revad-services:20080",
              "OCIS_REVA_DATA_ROOT": "/drone/src/tmp/reva/data/",
              "STORAGE_DRIVER": "OWNCLOUD",
              "SKELETON_DIR": "/drone/src/tmp/testing/data/apiSkeleton",
              "TEST_WITH_LDAP": "true",
              "REVA_LDAP_HOSTNAME": "ldap",
              "TEST_REVA": "true",
              "BEHAT_FILTER_TAGS": "~@notToImplementOnOCIS&&~@toImplementOnOCIS&&~comments-app-required&&~@federation-app-required&&~@notifications-app-required&&~systemtags-app-required&&~@provisioning_api-app-required&&~@preview-extension-required&&~@local_storage&&~@skipOnOcis-OC-Storage",
              "DIVIDE_INTO_NUM_PARTS": parallelRuns,
              "RUN_PART": runPart,
              "EXPECTED_FAILURES_FILE": "/drone/src/tests/acceptance/expected-failures-on-OWNCLOUD-storage.md",
            },
          },
        ],
        "services": [
          ldapService(),
          redisService(),
        ],
      }
    )

  return pipelines

def s3ngIntegrationTests(parallelRuns):
  pipelines = []
  for runPart in range(1, parallelRuns + 1):
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
            "image": "registry.cern.ch/docker.io/library/golang:1.16",
            "detach": True,
            "commands": [
              "cd /drone/src/tests/oc-integration-tests/drone/",
              "/drone/src/cmd/revad/revad -c frontend.toml &",
              "/drone/src/cmd/revad/revad -c gateway.toml &",
              "/drone/src/cmd/revad/revad -c shares.toml &",
              "/drone/src/cmd/revad/revad -c storage-home-s3ng.toml &",
              "/drone/src/cmd/revad/revad -c storage-oc-s3ng.toml &",
              "/drone/src/cmd/revad/revad -c storage-publiclink-s3ng.toml &",
              "/drone/src/cmd/revad/revad -c ldap-users.toml",
            ],
          },
          cloneOc10TestReposStep(),
          {
            "name": "oC10APIAcceptanceTestsS3ngStorage",
            "image": "registry.cern.ch/docker.io/owncloudci/php:7.4",
            "commands": [
              "cd /drone/src/tmp/testrunner",
              "make test-acceptance-api",
            ],
            "environment": {
              "TEST_SERVER_URL": "http://revad-services:20080",
              "OCIS_REVA_DATA_ROOT": "/drone/src/tmp/reva/data/",
              "DELETE_USER_DATA_CMD": "rm -rf /drone/src/tmp/reva/data/nodes/root/* /drone/src/tmp/reva/data/nodes/*-*-*-*",
              "STORAGE_DRIVER": "S3NG",
              "SKELETON_DIR": "/drone/src/tmp/testing/data/apiSkeleton",
              "TEST_WITH_LDAP": "true",
              "REVA_LDAP_HOSTNAME": "ldap",
              "TEST_REVA": "true",
              "BEHAT_FILTER_TAGS": "~@notToImplementOnOCIS&&~@toImplementOnOCIS&&~comments-app-required&&~@federation-app-required&&~@notifications-app-required&&~systemtags-app-required&&~@provisioning_api-app-required&&~@preview-extension-required&&~@local_storage&&~@skipOnOcis-OCIS-Storage",
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
      }
    )

  return pipelines
