Bugfix: Handle redirection prefixes when extracting destination from URL

The move function handler in ocdav extracts the destination path from the URL by
removing the base URL prefix from the URL path. This would fail in case there is
a redirection prefix. This PR takes care of that and it also allows zero-size
uploads for localfs.

https://github.com/cs3org/reva/pull/1111
