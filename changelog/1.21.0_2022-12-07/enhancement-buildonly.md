Enhancement: Migrate the BuildOnly job from Drone to GitHub Actions

We've migrated the BuildOnly job from Drone to GitHub Actions workflow.
The Workflow builds and Tests Reva, builds a Revad Docker Image and checks the license headers.
The license header tools was removed since the goheader linter provides the same functionality.

https://github.com/cs3org/reva/pull/3500