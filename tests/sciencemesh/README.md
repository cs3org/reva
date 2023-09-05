## ScienceMesh Development Setup

These scripts will create a Docker testnet which simulates the [ScienceMesh](https://sciencemesh.io).
It is useful for all kinds of ScienceMesh-related development and (manual) testing scenarios.

Prerequisites: bash, git, Docker.

```
git clone https://github.com/cs3org/reva
cd reva
cd examples/sciencemesh
./init-sciencemesh.sh # This will prepare the Nextcloud and ownCloud-10 images
./init-reva.sh # This will build reva and revad in the current repo and handle a few other prerequisites
./nrro.sh
./einstein.sh nextcloud1 # for owncloud1 / owncloud2, make sure to log in via the OC-10 GUI once before trying to access through reva-cli!
./maria2.sh
./clean.sh # Careful! This will kill and remove all your Docker containers on the current host system! Also unrelated ones if present.
./orro.sh
```

## Reva-to-reva

To initialize your development environment and build reva on the host, do:
```
./init-reva.sh # This will build reva and revad in the current repo and handle a few other prerequisites
# passing sleep as the main container command will allow us
# to run revad interactively later:
REVA_CMD="sleep 30000" ./scripts/testing-reva.sh
```

### Running the ocmd tutorial
After you've started the Docker testnet with container as above, you do:
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
After you've started the Docker testnet with container as above, you do:
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
