Bugfix: handle OCM disabled in getPermissionsByCs3Reference

When `OCMEnabled` is false, we should not query for OCM shares

https://github.com/cs3org/reva/pull/5328
