Enhancement: Extract the upload state machine into a driver-agnostic coordinator

The upload state machine (TUS session management, postprocessing event loop,
antivirus integration, and restart safety) has been extracted from decomposedfs
into a new coordinator in `pkg/upload`. Every storage driver now inherits TUS
chunked uploads, postprocessing, and AV scanning without reimplementing any of
it.

Drivers integrate by implementing four new methods on the storage interface:

- `MarkProcessing` sets or clears a "processing" flag on a resource so readers
  see a grayed-out placeholder while bytes are in flight. Drivers that do not
  need concurrent-upload protection may implement this as a no-op.
- `PrepareUpload` is called after all bytes are received and before
  postprocessing begins. Decomposedfs uses this to lock the node, snapshot the
  previous version, and propagate the optimistic size change. Drivers with no
  such requirements may return immediately.
- `CommitUpload` writes the staged bytes to the resource and receives
  pre-computed checksums.
- `RollbackUpload` is the inverse of `PrepareUpload` and is called when
  postprocessing fails or is aborted. Drivers that returned immediately from
  `PrepareUpload` may return nil. The `RollbackInfo` struct carries the node
  identity from the upload session rather than from live node metadata, so a
  rollback can still release the quota of a node whose metadata has become
  unreadable (e.g. because an ancestor was trashed mid-upload).

The coordinator owns the upload session files for the decomposedfs driver at
the same on-disk location as before (`<root>/uploads/`), so existing in-flight
uploads continue without interruption and no migration is required.

**Configuration:**

Both storageprovider and dataprovider gain an `upload_directory` config key that
sets the local directory where temporary upload session files and staged bytes are stored.
For decomposedfs this is optional; the coordinator falls back to `<root>/uploads/`
inside the driver's own root directory. For drivers that have no local filesystem
root, `upload_directory` must be set explicitly; otherwise the service fails to start.

The postprocessing consumer settings (`asyncfileuploads`, `consumer_group`,
`numconsumers`, `mount_id`) are read from the driver's own config block, the
same keys decomposedfs already uses. No new top-level config is introduced.

https://github.com/owncloud/reva/pull/702
https://github.com/owncloud/reva/pull/703
https://github.com/owncloud/reva/pull/714
https://github.com/owncloud/reva/pull/715
https://github.com/owncloud/reva/pull/717
https://github.com/owncloud/reva/pull/720
https://github.com/owncloud/reva/pull/721
