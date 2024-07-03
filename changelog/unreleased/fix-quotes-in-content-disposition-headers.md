Bugfix: Quotes in dav Content-Disposition header

We've fixed the the quotes in the dav `Content-Disposition` header. They caused an issue where certain browsers would decode the quotes and falsely prepend them to the filename.

https://github.com/cs3org/reva/pull/4748
https://github.com/owncloud/web/issues/11031
