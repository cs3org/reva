Bugfix: Do not delete mlock files

To prevent stale NFS file handles we no longer delete empty mlock files after updating the metadata.

https://github.com/cs3org/reva/pull/4936
https://github.com/cs3org/reva/pull/4924