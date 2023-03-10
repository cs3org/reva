Enhancement: Disable sharing on low level paths

Sharing can be disable in the user share provider
for some paths, but the storage provider
was still sending the sharing permissions for those paths.
This adds a config option in the storage provider,
`minimum_allowed_path_level_for_share`, to disable sharing
permissions for resources up to a defined path level.

https://github.com/cs3org/reva/pull/3717