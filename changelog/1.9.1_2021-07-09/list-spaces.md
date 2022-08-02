Enhancement: Introduce list spaces

The ListStorageSpaces call now allows listing all user homes and shared resources using a storage space id. The gateway will forward requests to a specific storage provider when a filter by id is given. Otherwise it will query all storage providers. Results will be deduplicated. Currently, only the decomposed fs storage driver implements the necessary logic to demonstrate the implmentation. A new `/dav/spaces` WebDAV endpoint to directly access a storage space is introduced in a separate PR.

https://github.com/cs3org/reva/pull/1802
https://github.com/cs3org/reva/pull/1803