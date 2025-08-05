Bugfix: remove fileid from link virtual folder

The front-end expects to not have a file ID on the root of a public link when it is a virtual folder around a single file share, for it to automatically open in the default app. The file id of this virtual folder has now been removed.

https://github.com/cs3org/reva/pull/5252