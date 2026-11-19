Bugfix: Close localfs prepared statements instead of leaking them

The localfs DB helpers prepared a statement and never closed it. Each call
therefore leaked one server-side prepared statement. Against MariaDB this
accumulates until the server refuses to create more:

    Error 1461 (42000): Can't create more than max_prepared_stmt_count
    statements (current value: 16382)

Once that limit is hit, the home-directory existence check fails while
statting the user's space, so authentication reports an error and every
DAV and graph request answers 401 even though the credentials are valid.
A restart clears it, which makes the failure look intermittent.

Every successful Prepare is now followed by a deferred Close, and a
regression test drives the DB helpers through a counting driver that wraps
the real sqlite driver and fails if Prepare and Close counts diverge.
