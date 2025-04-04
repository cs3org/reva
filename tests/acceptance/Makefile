.PHONY: help
help:
	@echo "Usage: make [target]"
	@echo "Available targets:"
	@echo "  reva            Start reva with decomposed storage"
	@echo "  reva-posix      Start reva with posixfs storage"
	@echo "  reva-decomposeds3       Start reva with decomposeds3 storage"

CURR_DIR:=$(dir $(abspath $(firstword $(MAKEFILE_LIST))))
REVA_CMD:=$(abspath $(CURR_DIR)/../../cmd/revad/revad)
CONFIG_DIR:=$(abspath $(CURR_DIR)/../oc-integration-tests/local)

.PHONY: ldap
ldap:
	@docker run --rm -d \
		--name docker-slapd \
		-e LDAP_TLS_VERIFY_CLIENT=never \
		-e LDAP_DOMAIN=example.org \
		-e LDAP_ORGANISATION="Example Organization" \
		-e LDAP_ADMIN_PASSWORD=admin \
		-p 389:389 \
		-p 636:636 \
		osixia/openldap:1.3.0

.PHONY: redis
redis:
	@docker run --rm -d \
		--name redis \
		-e REDIS_DATABASES=1 \
		-p 6379:6379 \
		redis:6-alpine

.PHONY: ceph
ceph:
	@docker run --rm -d \
		--name ceph \
		-e CEPH_DAEMON=demo \
		-e NETWORK_AUTO_DETECT=1 \
		-e MON_IP=0.0.0.0 \
		-e CEPH_PUBLIC_NETWORK=0.0.0.0/0 \
		-e RGW_NAME=localhost \
		-e CEPH_DEMO_UID=test-user \
		-e CEPH_DEMO_ACCESS_KEY=testadmin \
		-e CEPH_DEMO_SECRET_KEY=testadmin \
		-e CEPH_DEMO_BUCKET=test \
		-p 8080:8080 \
		ceph/daemon
	@echo "Waiting for ceph to be ready..."
	@while [ `curl -s http://localhost:8080 -w %{http_code} -o /dev/null` -ne 200 ]; do sleep 1; done

.PHONY: reva-services
reva-services:
	@$(REVA_CMD) -c $(CONFIG_DIR)/frontend.toml &
	@$(REVA_CMD) -c $(CONFIG_DIR)/gateway.toml &
	@$(REVA_CMD) -c $(CONFIG_DIR)/shares.toml &
	@$(REVA_CMD) -c $(CONFIG_DIR)/storage-shares.toml &
	@$(REVA_CMD) -c $(CONFIG_DIR)/storage-publiclink.toml &
	@$(REVA_CMD) -c $(CONFIG_DIR)/machine-auth.toml &
	@$(REVA_CMD) -c $(CONFIG_DIR)/permissions-opencloud-ci.toml &
	@$(REVA_CMD) -c $(CONFIG_DIR)/ldap-users.toml &

.PHONY: reva
reva: ldap reva-services
	@echo "Running reva with decomposed storage"
	@$(REVA_CMD) -c $(CONFIG_DIR)/storage-users.toml

.PHONY: reva-posix
reva-posix: ldap redis reva-services
	@echo "Running reva with posixfs storage"
	@$(REVA_CMD) -c $(CONFIG_DIR)/storage-users-posixfs.toml

.PHONY: reva-decomposeds3
reva-decomposeds3: ldap ceph reva-services
	@echo "Running reva with decomposeds3 storage"
	@$(REVA_CMD) -c $(CONFIG_DIR)/storage-users-decomposeds3.toml

.PHONY: clean
clean:
	@docker ps -q --filter "name=docker-slapd" | xargs -r docker stop
	@docker ps -q --filter "name=redis" | xargs -r docker stop
	@docker ps -q --filter "name=ceph" | xargs -r docker stop
	@pkill revad || true
	@rm -rf /tmp/reva
