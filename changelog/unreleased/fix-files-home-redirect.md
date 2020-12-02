Bugfix: Dav endpoint routing to home storage when request is remote.php/dav/files

There was a regression in which we were not routing correctly to the right storage depending on the url.

https://github.com/cs3org/reva/pull/1342