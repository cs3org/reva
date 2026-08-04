Bugfix: Retry LDAP operations that fail while sending the request

An operation on an idle connection that was reaped by a server idle timeout, load balancer or
firewall could fail without being retried. When the operation reached go-ldap's write loop before
its reader goroutine noticed the drop, `IsClosing()` was still false and the failure surfaced from
`processMessages` as `unable to send request: ...`. That is a plain error rather than an
`*ldap.Error`, so it carries no result code and mapped to `LDAPResultOther`, which neither the read
nor the write retry policy treated as retryable.

`isSendFailedErr` now matches that failure by message, and both policies retry it. It is safe to
retry a write: go-ldap adds a message to `messageContexts` only after `conn.Write` succeeds, so an
operation failing here provably never reached the server and cannot be double-applied. Errors raised
after the request was transmitted (a drop while reading the response, or a request timeout) are still
never retried for writes.

`ConnPool.release` also evicts a connection on this error instead of returning it to the idle pool,
where it would have failed the next checkout that picked it up.

https://github.com/owncloud/reva/pull/678
