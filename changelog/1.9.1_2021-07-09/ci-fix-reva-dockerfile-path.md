Bugfix: correct Dockerfile path for the reva CLI and alpine3.13 as builder

This was introduced on https://github.com/cs3org/reva/commit/117adad while
porting the configuration on .drone.yml to starlark.

Force golang:alpine3.13 as base image to prevent errors from Make when
running on Docker <20.10 as it happens on Drone
 ref.https://gitlab.alpinelinux.org/alpine/aports/-/issues/12396

https://github.com/cs3org/reva/pull/1843
https://github.com/cs3org/reva/pull/1844
https://github.com/cs3org/reva/pull/1847
