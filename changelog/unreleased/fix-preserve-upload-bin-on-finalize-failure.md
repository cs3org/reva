Bugfix: Preserve upload bin when synchronous Finalize fails

When a synchronous upload finalization (e.g. storage-system with NFS blobstore) called
Finalize and WriteBlob failed, Cleanup was invoked with cleanBin=true unconditionally.
This permanently deleted the bin file from uploads/ even though the blob never reached
blobs/, making it unrecoverable — including via the move-stuck-upload-blobs CLI.

Fix: only clean the bin file when Finalize succeeds (err == nil), so a failed upload
bin is preserved for manual recovery.

https://github.com/owncloud/reva/pull/577
