Enhancement: administer quotas and routes through the Admin API

The storage provider exposes admin invocations to read and set the quota of a
space it serves (the project quota of a path, or the quota of a user), through
a new storage.QuotaAdmin interface the EOS driver implements; it also
advertises its mount in the registry metadata so a client can pick the
instance serving a path. The storage registry exposes invocations to list the
providers, resolve a path, and, with the dynamic driver, list, add, remove and
reload the routes: the routing table used to be read once at start-up, so a
change needed a restart to take effect.

https://github.com/cs3org/reva/pull/5902
