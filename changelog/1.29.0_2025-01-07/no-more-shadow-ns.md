Enhancement: drop shadow namespaces

This comes as part of the effort to operate EOS without being root, see https://github.com/cs3org/reva/pull/4977

In this PR the post-home creation hook (and corresponding flag) is replaced by a create_home_hook, and the following configuration parameters are suppressed:

    shadow_namespace
    share_folder
    default_quota_bytes
    default_secondary_quota_bytes
    default_quota_files
    uploads_namespace (unused)

https://github.com/cs3org/reva/pull/4984