Bugfix: return 404 when no permission to space

WebDAV expects a 409 response when trying to upload into a non existing folder. We fixed the implementation to return 404 when a user has no access to a space and still return a 409 when a parent folder does not exist (and he has access to the space).

https://github.com/cs3org/reva/pull/3368
https://github.com/cs3org/reva/pull/3300
https://github.com/owncloud/ocis/issues/3561
