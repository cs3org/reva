Enhancement: allow multi-user share in OCS

Sending multiple POST requests for multiple users leads to parallel calls to EOS, which suffers from a critical race condition when setting ACLs. So, now the reva OCS endpoint supports sending multiple comma-seperated users.

https://github.com/cs3org/reva/pull/5235