Bugfix: Block overwriting mountpoints

This blocks overwriting mountpoints through the webdav COPY api. It is now returning a bad request when attempting to overwrite a mountpoint.

https://github.com/cs3org/reva/pull/4785
