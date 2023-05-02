Enhancement: Check set project space quota permission

Instead of checking for `set-space-quota` we now check for `Drive.ReadWriteQuota.Project` when changing project space quotas.

https://github.com/cs3org/reva/pull/3690
