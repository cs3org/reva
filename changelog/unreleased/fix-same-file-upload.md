Bugfix: Uploading the same file multiple times leads to orphaned blobs

Fixed a bug where multiple uploads of the same file would lead to orphaned blobs in the blobstore. These orphaned blobs will now be deleted.

https://github.com/cs3org/reva/pull/4746
