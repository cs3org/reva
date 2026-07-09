Enhancement: Add config option to disable public link sharing

Added an `enable_public_sharing` config option to the publicshareprovider
service. When set to `false`, creating new public links is rejected
(internal links, which carry no permissions, are unaffected), and the
`files_sharing.public.enabled` capability reports `false`. A hardcoded
override that always forced `files_sharing.public.enabled` to `true` was
also removed so the capability reflects the actual configured value.

https://github.com/owncloud/reva/pull/651
