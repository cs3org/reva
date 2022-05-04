Bugfix: Dockerfile.revad-ceph to use the right base image

In Aug2021 https://hub.docker.com/r/ceph/daemon-base was moved to quay.ceph.io
and the builds for this image were failing for some weeks after January.

https://github.com/cs3org/reva/pull/2588
