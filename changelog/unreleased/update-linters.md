Bugfix: Update the linters

The deprecated linters 'scopelint', 'golint' and maligned' are deprecated and replaced by 'exportloopref', 'revive' and govet 'fieldalignment' respectively.
Bumped golangci-lint version to v1.45.2

https://github.com/cs3org/reva/issues/2319
https://github.com/cs3org/reva/pull/2770
