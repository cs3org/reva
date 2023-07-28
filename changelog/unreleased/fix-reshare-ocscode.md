Bugfix: Fix ocs status code for 'No share permission' response

Request to re-share a resource by a user who does not have share permission on the resource returned a 404 status code.
This is fixed and a 403 status code is returned instead.

https://github.com/owncloud/ocis/issues/6670
https://github.com/cs3org/reva/pull/4086
