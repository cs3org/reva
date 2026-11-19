Enhancement: Cap the localfs database connection pool

localfs opened its sqlite database with sql.Open and never set a pool
limit, so the pool was unbounded: every connection the driver opened stayed
available and nothing bounded the count.

That matters because a leaked result set does not just leak a statement. In
database/sql there is no finalizer on Rows, and only Rows.close calls
releaseConn, so an abandoned result set holds its connection until the
process exits. With no cap, a single leak path can therefore consume
connections without limit until the process runs out of file descriptors.

Observed on a deployment with the statement leak already fixed: driving one
remaining leak path exhausted MariaDB with "Error 1040 (08004): Too many
connections", after which even a plain client login failed and every
request answered 401.

The pool is now capped at 8 connections, which is enough for the concurrent
Stat traffic localfs serves. The sqlite driver waits up to 5s for a lock by
default, so the cap does not turn contention into errors. Measured with 64
concurrent readers and a mixed read/write workload: no failures and no lock
timeouts at this size.

This does not replace fixing leaks, and the leak paths themselves are fixed
separately. It makes the failure mode bounded: a future leak now slows the
service instead of taking down authentication for every user.

https://github.com/cs3org/reva/pull/
