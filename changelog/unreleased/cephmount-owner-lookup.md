Enhancement: Lookup owners via the gateway

Previously ceph had been looking up owners via `sssd` and
this is very slow so now we do the same thing for ceph and change
the lookups to go via the gateway to speed things up where possible.

https://github.com/cs3org/reva/pull/6046