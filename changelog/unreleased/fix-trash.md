Bugfix: restore and delete trash items via ocs

The OCS api was not passing the correct key and references to the CS3 API. Furthermore, the owncloud storage driver was constructing the wrong target path when restoring.

https://github.com/cs3org/reva/pull/1103