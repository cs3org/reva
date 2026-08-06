Bugfix: Release the quota of upload sessions with unreadable node metadata

When an upload's target node lost its metadata, e.g. because an ancestor was
moved to the trash while the upload was still in flight, the node file remained
on disk without a readable `.mpk`. Reading such a node fails with
`Missing parent ID on node`, so the upload could never finish postprocessing. It
stayed in "Processing" forever, could not be downloaded or deleted, and kept
consuming the space quota.

Cleaning these sessions up did not work either. `Cleanup` removed the upload
bytes and the session info file *before* attempting to revert the node, then
bailed out on the failing node read without ever releasing the quota. That
destroyed both the only copy of the uploaded data and the session metadata
needed to repair the node, while freeing nothing.

Cleanup now reverts the node before removing anything irreversible and falls
back to the parent id recorded in the session when the node metadata cannot be
read, so the quota is released and the orphaned node is removed. If the quota
cannot be released the upload is kept so it can be retried instead of being lost.
Sessions whose node is unreadable are now also cleaned up when postprocessing
finishes, instead of being left behind to be retried indefinitely.

A new `Orphaned` upload session filter allows listing the affected sessions. It
is only evaluated when set, as it reads the node metadata of every session.

https://github.com/owncloud/reva/pull/692
