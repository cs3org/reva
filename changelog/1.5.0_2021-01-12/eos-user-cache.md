Enhancement: Add cache to store UID to UserID mapping in EOS

Previously, we used to send an RPC to the user provider service for every lookup
of user IDs from the UID stored in EOS. This PR adds an in-memory lock-protected
cache to store this mapping.

https://github.com/cs3org/reva/pull/1340
