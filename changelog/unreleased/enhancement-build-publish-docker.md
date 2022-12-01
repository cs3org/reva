Enhancement: Migrate the buildAndPublishDocker job from Drone to GitHub Actions

We've migrated the buildAndPublishDocker job from Drone to GitHub Actions workflow.
We've updated the Golang version used to build the Docker images to go1.19.
We've fixed the Cephfs storage module.
We've improved the Makefile.
We've refactored the build-docker workflow.

https://github.com/cs3org/reva/pull/3506