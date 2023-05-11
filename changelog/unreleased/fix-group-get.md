Bugfix: Fix group request to Grappa

The `url.JoinPath` call was returning an url-encoded string, turning `?` into
`%3`. This caused the request to return 404.

https://github.com/cs3org/reva/pull/3883
