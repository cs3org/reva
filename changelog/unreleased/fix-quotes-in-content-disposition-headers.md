Bugfix: Quotes in dav Content-Disposition header

We've fixed the placement of the quotes in the dav `Content-Disposition` header. The misplacement caused an issue where certain browsers would decode the quotes and falsely prepend them to the filename.

https://github.com/cs3org/reva/pull/4747
https://github.com/owncloud/web/issues/11031
