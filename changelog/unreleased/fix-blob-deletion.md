Bugfix: Actually remove blobs when purging

Blobs were not being deleted properly on purge.
Now if a folder gets purged all its children will be deleted

https://github.com/cs3org/reva/pull/2868
