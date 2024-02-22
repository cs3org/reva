Bugfix: the sharemanager can now reject grants with resharing permissions

When disabling resharing we also need to prevent grants from allowing any grant permissions.

https://github.com/cs3org/reva/pull/4516
