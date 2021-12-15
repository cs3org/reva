Bugfix: Remove early finish for zero byte file uploads

We've fixed the upload of zero byte files by removing the
early upload finishing mechanism.

https://github.com/cs3org/reva/issues/2309
https://github.com/owncloud/ocis/issues/2609
