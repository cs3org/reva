Bugfix: Close result sets in localfs and the dynamic registry

Two more paths leaked database resources in the same way as the prepared
statement leak. A helper that returns *sql.Rows to its caller does not
release the result set unless the caller closes it, and an open result set
keeps the underlying statement alive on the server.

- localfs: retrieveArbitraryMetadata and ListGrants now close the rows they
  receive from getMetadata and getACLs. These run on every Stat and every
  ListGrants call, so the leak grew with normal browsing traffic.
- dynamic registry: initRoutingTree never closed the routing rows nor the
  database handle it opened. This one runs once at startup, so the impact is
  small, but it is the same defect.

Both packages now also check Rows.Err, which was previously ignored and
turned a mid-iteration error into a silently truncated result.

Found with sqlclosecheck and rowserrcheck, which report nothing left in the
tree. TestResultSetsAreClosed covers the early-return path that actually
leaks: iterating to completion closes the rows implicitly, so the test
forces a Scan failure and asserts the result set is still released.

https://github.com/cs3org/reva/pull/6035
