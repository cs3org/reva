Bugfix: Ensure ignoring public stray shares

When using the json public shares manager, it can be the case we found a share with a resource_id that no longer exists.

https://github.com/cs3org/reva/pull/1090