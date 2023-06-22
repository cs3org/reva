Bugfix: Close archive writer properly

When running into max size error (or random other error) the archiver would not close the writer. Only it success case it would.
This resulted in broken archives on the client. We now `defer` the writer close.

https://github.com/cs3org/reva/pull/4007
