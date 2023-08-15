Bugfix: Fix downloading remote files gives HTTP 500 error in owncloud

I've fixed the problem when ownCloud attempts to download a file from the remote site with reva in between, the download operation fails with HTTP 500.

https://github.com/cs3org/reva/pull/4112
https://github.com/cs3org/reva/issues/4068
