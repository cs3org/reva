Enhancement: Reduce the size of all the container images built on CI

Previously, all images were based on golang:1.16 which is built from Debian.
Using 'scratch' as base, reduces the size of the artifacts well as the attack
surface for all the images, plus copying the binary from the build step ensures
that only the strictly required software is present on the final image.
For the revad images tagged '-eos', eos-slim is used instead. It is still large
but it updates the environment as well as the EOS version.

https://github.com/cs3org/reva/pull/1705
