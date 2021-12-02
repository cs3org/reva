Bugfix: Fix status code for WebDAV mkcol requests where an ancestor is missing

We've fixed the status code to 409 according to the WebDAV standard for MKCOL
requests where an ancestor is missing. Previously these requests would fail
with an different error code (eg. 500) because of storage driver limitations
(eg. oCIS FS cannot handle recursive creation of directories). 

https://github.com/owncloud/ocis/issues/2767
https://github.com/cs3org/reva/pull/2293
