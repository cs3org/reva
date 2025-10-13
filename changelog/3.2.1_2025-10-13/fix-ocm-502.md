Bugfix: fix 502 for LW accs when OCM is enabled

When OCM is enabled, going to `/sharedWithMe` or `/sharedByMe` results in a 502.
We now skip checking for OCM shares on LW accounts. Additionally, error handling in the
libregraph layer has been improved

https://github.com/cs3org/reva/pull/5345
