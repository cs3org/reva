Enhancement: Fix remaining quota calculation

Remaining quota should only be total - used and not take disk space into account.

https://github.com/cs3org/reva/pull/4897
