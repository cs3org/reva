Bugfix: Do not stat shared resources when downloading

Previously, we statted the resources in all download requests resulting in
failures when downloading references. This PR fixes that by statting only in
case the resource is not present in the shares folder. It also fixes a bug where
we allowed uploading to the mount path, resulting in overwriting the user home
directory.

https://github.com/cs3org/reva/pull/1038
