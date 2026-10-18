Enhancement: send OCM notifications to peers that support them

The OCM client's `NewNotification` stub is replaced by a typed sender. It
discovers the peer, with the existing /ocm-provider fallback and 1 MiB
response limit, and posts unsigned JSON exactly once, only when the peer
advertises the `notifications` capability. Any 2xx response counts as
delivered and its body is ignored; 400, 401, 403, 404, 501 and 503 are
classified and logged, redirects are not followed, and nothing is retried,
queued or re-published. A failed send never undoes the change that caused it.
Share-state notices carry notification.file.providerId, the optional
notification.sharedSecret (the outgoing token for SHARE_UNSHARED, the received
secret for SHARE_ACCEPTED and SHARE_DECLINED) and, as a temporary deviation
for peers still on the older Nextcloud receiver, the deprecated top-level
providerId. The secret is never logged.
