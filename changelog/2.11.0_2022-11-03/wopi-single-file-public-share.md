Bugfix: Fix wopi access to publicly shared files

Wopi requests to single file public shares weren't properly authenticated.
I added a new check to allow wopi to access files which were publicly shared.

https://github.com/cs3org/reva/pull/3257
https://github.com/owncloud/ocis/issues/4382
