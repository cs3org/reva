Bugfix: init admin connection instead of mounting

The ceph admin connection was mounted instead of initialized, which worked
when the key had mounting permissions but it does not anymore

https://github.com/cs3org/reva/pull/5825