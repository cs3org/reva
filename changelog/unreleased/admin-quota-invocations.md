Enhancement: space quota invocations on the storage provider

The storage provider has the admin invocations `quota_get`, `quota_set` and
`quota_list`. They take the root, the type and the owner of a space, and the
storage driver chooses how the storage keeps the quota. The EOS driver uses the
project quota (gid 99) on the root of the space, and reads the user quota of
the owner for older spaces. The new EOS option `quota_replica_factor` converts
the logical bytes of an operator into the physical bytes that EOS sets. The
storage provider publishes its `mount_id` and `mount_path` in the registry
metadata. The EOS gRPC client no longer needs a uid to set a project quota.

https://github.com/cs3org/reva/pull/XXXX
