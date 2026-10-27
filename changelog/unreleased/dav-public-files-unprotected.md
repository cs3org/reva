Bugfix: Allow unauthenticated access to /dav/public-files/

ownCloud Web no longer prefixes WebDAV paths with `remote.php`, so it now resolves
public links through `/dav/public-files/<token>`. Only `/remote.php/dav/public-files/`
was registered as an unprotected path, so the auth middleware rejected the first,
unauthenticated request with an empty 401 before it reached ocdav. Without the
`ERR_MISSING_BASIC_AUTH` error code in the response body, the web client could not
tell whether a password was required and failed to open public links.
`/dav/public-files/` is now unprotected as well.

https://github.com/cs3org/reva/pull/5965
