Bugfix: removed stat to all storage providers on Depth:0 PROPFIND to "/"

This PR removes an unnecessary and potentially problematic call, which would
fail if any of the configured storage providers has an issue.

https://github.com/cs3org/reva/pull/4497
