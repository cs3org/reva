Bugfix: Fix propfinds with depth 0

Fixed the response for propfinds with depth 0. The response now doesn't contain the shares jail anymore.

https://github.com/owncloud/ocis/issues/3704
https://github.com/cs3org/reva/pull/2918
