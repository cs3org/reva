Enhancement: notify OCM peers after committed share changes

The OCM share provider now sends one notification after a share change is
committed. Deleting an outgoing share sends SHARE_UNSHARED to the grantee's
server, and changing a stored WebDAV or webapp permission sends
SHARE_CHANGE_PERMISSION; expiration-only, app-name-only and unchanged updates
send nothing. Accepting or explicitly declining a received share sends
SHARE_ACCEPTED or SHARE_DECLINED to the server that sent it. For embedded
shares, SHARE_ACCEPTED is only sent when the transfer completes, and a failed
transfer is not reported as a decline.

The share repositories now return receipts read inside each mutation, so
repeated or concurrent identical changes notify at most once, and a failed
mutation notifies nothing. Embedded transfers carry an attempt generation
stored with the received share: a second start is refused while one runs, and
a completion only applies to the attempt that is still current, writing the
state and mtime only. Sending happens once, in the background, only to peers
advertising notifications; a delivery failure is logged and never undoes the
change. Creating a share and removing a contact still send no notification.
