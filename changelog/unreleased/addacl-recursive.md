Bugfix: make AddACL only recursive on directories

The current implementation of AddACL in the EOS gRPC client always sets msg.Recursive = true. This causes issues on the EOS side, because it will try running a recursive find on a file, which fails. This PR fixes this bug in Reva.

https://github.com/cs3org/reva/pull/4898