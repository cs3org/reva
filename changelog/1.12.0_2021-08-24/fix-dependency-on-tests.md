Bugfix: fix dependency on tests

The Nextcloud storage driver depended on a mock http client from the tests/ folder
This broke the Docker build
The dependency was removed
A check was added to test the Docker build on each PR

https://github.com/cs3org/reva/pull/2000
