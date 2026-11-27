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

This leak is not only about prepared statements. database/sql puts no
finalizer on Rows, and only Rows.close calls releaseConn, so an abandoned
result set holds its connection for the lifetime of the process. localfs
opens the database with sql.Open and never caps the pool, so the leak
exhausts connections before it exhausts statements. Observed on a build
that had the statement fix but not this one: driving the Scan-error path
in retrieveArbitraryMetadata 100 times left MariaDB answering
"Error 1040 (08004): Too many connections" to every client, including
the home-directory existence check, which turned every DAV and graph
request into a 401. The 1461 error reported in the statement fix is the
same leak observed further along when a statement limit is hit first.

Found with sqlclosecheck and rowserrcheck, which report nothing left in the
tree. TestResultSetsAreClosed covers the early-return path that actually
leaks: iterating to completion closes the rows implicitly, so the test
forces a Scan failure and asserts the result set is still released.

https://github.com/cs3org/reva/pull/6035
