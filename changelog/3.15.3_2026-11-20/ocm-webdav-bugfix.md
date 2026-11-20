Bugfix: Don't use the default gowebdav auth

When using the default gowebdav authentication the first request is sent
twice, to limit the number of requests this is now fixed.

https://github.com/cs3org/reva/pull/5979