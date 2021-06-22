Bugfix: Set CGO_ENABLED flag for 'scratch' based docker builds

The CGO_ENABLED=0 flag was added to the docker build flags so that it will
produce a static build. This allows usage of the 'scratch' image for
reduction of the docker image size.

https://github.com/cs3org/reva/issues/1765
https://github.com/cs3org/reva/pull/1766