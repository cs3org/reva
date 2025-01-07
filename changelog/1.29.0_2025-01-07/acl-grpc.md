Bugfix: make ACL operations work over gRPC

This change solves two issues: 
* AddACL would fail, because the current implementation of AddACL in the EOS gRPC client always sets msg.Recursive = true. This causes issues on the EOS side, because it will try running a recursive find on a file, which fails. 
* RemoveACL would fail, because it tried matching ACL rules with a uid to ACL rules with a username. This PR changes this approach to use an approach similar to what is used in the binary client: just set the rule that you want to have deleted with no permissions.

https://github.com/cs3org/reva/pull/4898