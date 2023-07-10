## ScienceMesh Development Setup

(under construction!)

These scripts will create a Docker testnet which simulates the [ScienceMesh](https://sciencemesh.io).
It is useful for all kinds of ScienceMesh-related development and (manual) testing scenarios.

Prerequisites: bash, git, Docker.

```
git clone --branch=sciencemesh-testing https://github.com/cs3org/reva
cd reva
cd examples/sciencemesh
./init-sciencemesh.sh # This will build reva and revad in the current repo
./nrro.sh
./clean.sh # Careful! This will kill and remove all your Docker containers on the current host system! Also unrelated ones if present.
./orro.sh
```

## Reva-to-reva
To initialize your development environment and build reva on the host, do:
```
./scripts/init-reva.sh
# passing sleep as the main container command will allow us
# to run revad interactively later:
REVA_CMD="sleep 30000" ./scripts/testing-reva.sh
docker exec -it revad1.docker bash
> cd /reva
> git config --global --add safe.directory /reva
> make revad
> make reva
```

### Running the ocmd tutorial
After you've run `make revad` and `make reva` once in one of the two containers as detailed above, you do:
* `docker exec -it revad1.docker bash` and then:
```
cd /etc/revad/ocmd
/reva/cmd/revad/revad -dev-dir server-1
```
* `docker exec -it revad2.docker bash` and then:
```
cd /etc/revad/ocmd
/reva/cmd/revad/revad -dev-dir server-2
```
* `docker exec -it revad1.docker bash` again for `/reva/cmd/reva/reva -insecure -host localhost:19000` etc.
* `docker exec -it revad2.docker bash` again for `/reva/cmd/reva/reva -insecure -host localhost:17000` etc. (notice the port number!)
* follow the rest of https://reva.link/docs/tutorials/share-tutorial/

### Running the datatx tutorial
After you've run `make revad` and `make reva` once in one of the two containers as detailed above, you do:
* `docker exec -it revad1.docker bash` and then:
```
cd /etc/revad/datatx
/reva/cmd/revad/revad -dev-dir server-1
```
* `docker exec -it revad2.docker bash` and then:
```
cd /etc/revad/datatx
/reva/cmd/revad/revad -dev-dir server-2
```
* `docker exec -it revad1.docker bash` again for `/reva/cmd/reva/reva -insecure -host localhost:19000` etc.
* `docker exec -it revad2.docker bash` again for `/reva/cmd/reva/reva -insecure -host localhost:17000` etc. (notice the port number!)
* get einstein to generate an invite, and marie to accept it, following the usual way as described in https://reva.link/docs/tutorials/share-tutorial/#4-invitation-workflow
* follow the rest of https://reva.link/docs/tutorials/datatx-tutorial/#3-create-a-datatx-protocol-type-ocm-share
