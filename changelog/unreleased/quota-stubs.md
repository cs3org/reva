Bugfix: Add quota stubs

The `owncloud` and `owncloudsql` drivers now read the available quota from disk to no longer always return 0, which causes the web UI to disable uploads.

https://github.com/cs3org/reva/pull/1985