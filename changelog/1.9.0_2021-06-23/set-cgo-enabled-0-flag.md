Bugfix: 'golang:alpine' as base image & CGO_ENABLED just for the CLI

Some of the dependencies used by revad need CGO to be enabled in order to
work. We also need to install the 'mime-types' in alpine to correctly
detect them on the storage-providers.

The CGO_ENABLED=0 flag was added to the docker build flags so that it will
produce a static build. This allows usage of the 'scratch' image for
reduction of the docker image size (e.g. the reva cli).

https://github.com/cs3org/reva/issues/1765
https://github.com/cs3org/reva/pull/1766
https://github.com/cs3org/reva/pull/1797
