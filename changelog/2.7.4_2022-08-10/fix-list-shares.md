Bugfix: Check ListGrants permission when listing shares

We now check the ListGrants permission when listing outgoing shares. If this permission is set, users can list all shares in all spaces.

https://github.com/cs3org/reva/pull/3141
