Enhancement: Introduce new webdav spaces endpoint

Clients can now use a new webdav endpoint `/dav/spaces/<storagespaceid>/relative/path/to/file` to directly access storage spaces.

The `<storagespaceid>` can be retrieved using the ListStorageSpaces CS3 api call.

https://github.com/cs3org/reva/pull/1803
