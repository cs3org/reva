Bugfix: fix owncloudsql GetMD

The GetMD call internally was not prefixing the path when looking up resources by id.

https://github.com/cs3org/reva/pull/1993