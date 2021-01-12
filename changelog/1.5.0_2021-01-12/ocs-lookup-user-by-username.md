Bugfix: when sharing via ocs look up user by username

The ocs api returns usernames when listing share recipients, so the lookup when creating the share needs to search the usernames and not the userid.

https://github.com/cs3org/reva/pull/1281
