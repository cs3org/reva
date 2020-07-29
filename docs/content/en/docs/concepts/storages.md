---
title: "Storage Drivers"
linkTitle: "Storage Drivers"
weight: 10
description: >
  Storage drivers and their capabilities
---

## Aspects of storage drivers
A lot of different storage technologies exist, ranging from general purpose file systems to software defined storage. Choosing any of them is making a tradeoff decision. Or, if a storage technology is already in place it automatically predetermines the capabilities that reva can make available via the CS3 API. *Not all storage systems are created equal.*

The CS3APIS connect Storage and Applications Providers, allowing them to exchange information about various aspects of storage.

### Tree persistence
An important aspect of a filesystem is organizing files and directories in some form of hierarchy. 
Using the CS3 API you can manage this hierarchy by creating, moving and deleting nodes. Beside the name a node also has well known metadata like size and mtime that are managed by tree as well.

While traditionally nodes in the tree are reached by traversing the path the tree persistence should be prepared to look up a node by an id. Think of an inode in a POSIX filesystem. If this operation needs to be cached for performance reasons keep in mind that cache invalidation is hard and crawling all files to update the inode to path mapping takes O(n), not O(1).

Depending on the underlying storage technology some operations may either be slow, up to a point where it makes more sense to disable them entirely. One example is a folder rename: on S3 a *simple* folder rename translates to a copy and delete operation for every child of the renamed folder. There is an exception though: this restriction only applies if the S3 storage is treated like a filesystem, where the keys are the path and the value is the file content. There are smarter ways to implement file systems on top of S3, but again: there is always a tradeoff.

> **Folders are not directories**
> There is a difference between *folder* and *directory*: a *directory* is a file system concept. A *folder* is a metaphor for the concept of a physical file folder. There are also *virtual folders* or *smart folders* like the recent files folder which are no file system *directories*. So, every *directory* and every *virtual folder* is a *folder*, but not every *folder* is a *directory*. See [the folder metaphor in wikipedia](https://en.wikipedia.org/wiki/Directory_(computing)#Folder_metaphor)

### Arbitrary Metadata persistence
In addition to well known metadata users might be able to add arbitrary metadata like tags, comments or [dublin core](https://en.wikipedia.org/wiki/Dublin_Core) using the CS3 API.

### Data persistence
While File up and download are not part of the CS3 API they can be initiated with it. Initiation responses contain the target URL and allow clients to switch to a more suitable protocol. For download a normal GET request might be sufficient. For upload a resumable protocol like [tus.is](https://tus.io/) might make more sense.

### Path Mapping
While a storage driver presents a file hierarchy to the API consumer it might organize the internal layout of the tree in a different manner. For example it might add the logged in users name to the path, introduce additional sub folders to organize files and versions in the same tree or even completely deconstruct the path and work on file ids.

### Trash persistence
With the CS3 API files can be restored from a trash, if the underlying storage technology supports it, or if a special file layout is used to implement it. In the latter case, all delete operations must move files to the trash location if they should be visible using the CS3 API. If you bypass the CS3 API and delete the file without moving it to the trash location (as in ssh to the storage and `rm` the file), the data is gone.

### Versions persistence
When the underlying storage technology supports it, the CS3 API also allows listing and restoring file versions. Capturing file versions is harder than a trash, because every file change must be recorded. Similar to the trash this can be done by a storage driver in reva, but when bypassing it versions will not be recorded, unless the storage technology itself has versioning support.

### Grant persistence
The CS3 API uses grants to describe access permissions. Storage systems have a wide range of permissions granularity and not all grants may be supported by every storage driver. If the storage system does not support certain grant properties, eg. expiry, then the storage driver may choose to implement them in a different way. Expiries could be persisted in a different way and checked periodically to remove the grants. Again: every decision is a tradeoff.

### ETag propagation
An important aspect when considering the CS3 API for synchronization is that there is no delta API, yet. A client can however discover changes by recursively descending the tree and comparing the ETag for every node. If the storage technology supports propagating ETag changes up the tree, only the root node of a tree needs to be checked to determine if a discovery needs to be started and which nodes need to be traversed. This allows using the storage technology itself to persist all metadata that is necessary for sync, without additional services or caches.

### Activity History
Building an activity history requires tracking the different actions that have been performed, at least using the CS3 API, but preferably on the storage itself. This does not only include file changes but also metadata changes like renames and permission changes. Maybe even public link access.

Since the majority of these actions are already persisted by the versions history it makes more sense to keep track of these events in an external append only data structure to efficiently add and query events, which is why an activity history is not part of the CS3 API.

## Storage driver implementations in reva

Reva comes with a set of storage driver implementations. The following sections will list the known tradeoffs.

### Local Storage Driver
- naive implementation for a local POSIX filesystem
- no ETag propagation
  - could be done with an external inotify like mechanism
- path by id lookup is not efficient
  - uses file path as id, so ids are not stable
- no trash
- no versions

### EOS Storage Driver
- requires EOS as the storage
- relies on the EOS native ETag propagation
- supports EOS native trash and versions
- path by id lookup is efficient
- bypassing the driver is possible because all operations are implemented in EOS natively

### OwnCloud Storage Driver
- uses the ownCloud 10 data directory layout
- implements ETag propagation
- path by id lookup is efficient
  - uses redis for the mapping
- supports trash and versions when using the driver
  - limitations for trash file length
  - limitations for versions file length
- requires redis for efficient file id to path lookup

### S3 Storage Driver
- this implementation assumes keys reflect the path
- inefficient move operation, because every file has to be copied and deleted
- no ETag propagation
  - if the storage technology supports notifications they could be used to update parent ETags
- uses file path as id, so ids are not stable
- trash not implemented, yet
- versions not implemented, yet

## The reva Storage Provider
The above storage drivers can be used in reva by configuring a [storageprovider](../../config/grpc/services/storageprovider/) or [dataprovider](../../config/http/services/dataprovider/).