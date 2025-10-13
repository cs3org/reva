Enhancement: List OCM shares in LibreGraph implementation

* Adapating the LibreGraph implementation to also list OCM shares
* Bugfix for uploading (e.g. editing files) through OCM, headers had to be updated
* Bugfix where stating a non existing file did not return a not found error - this made CreateDir and Touch fail since reva could not determine if the file exists.
* Bugfix for creating files and directories through OCM, essentially the old logic was to try and stat the file to test different the different authentication methods basic and bearer, however this doesn't work for Touch and CreateDir since the stat is meant the fail here, the logic needs to be looked over. Further the OCM basic auth over webdav is broken and this causes these two operations to fail - since the stat fails and it reverts to basic auth which fails since it is currently broken.

https://github.com/cs3org/reva/pull/5316