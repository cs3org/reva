Enhancement: Add HTTP header to disable versioning on EOS

This enhancement introduces a new header, `X-Disable-Versioning`, on PUT requests. EOS will not version this file save whenever this header is set with a truthy value.
See also: https://github.com/cs3org/reva/pull/4864