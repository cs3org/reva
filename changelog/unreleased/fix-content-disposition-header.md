Bugfix: Fix Content-Disposition header in dav

We have added missing quotes to the Content-Disposition header in the dav service. This fixes an issue with files containing special characters in their names.

https://github.com/cs3org/reva/pull/4498
https://github.com/owncloud/ocis/issues/8361
