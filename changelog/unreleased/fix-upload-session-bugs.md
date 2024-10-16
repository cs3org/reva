Bugfix: Fix upload session bugs

We fixed an issue that caused a panic when we could not open a file to calculate checksums. Furthermore, we now delete the upload session .lock file on cleanup.

https://github.com/cs3org/reva/pull/4888
