Bugfix: return etag for ocm shares

The ocm remote storage now passes on the etag returned in the PROPFIND response.

https://github.com/cs3org/reva/pull/4823
https://github.com/owncloud/ocis/issues/9534
