Enhancement: Storage registry: fail at init if config is missing any providers

This change makes the dynamic storage registry fail at startup if there are
missing rules in the config file. That is, any `mount_id` in the routing table
must have a corresponding `storage_id`/`address` pair in the config, otherwise
the registry will fail to start.

https://github.com/cs3org/reva/pull/4370
