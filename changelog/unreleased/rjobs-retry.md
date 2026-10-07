Enhancement: let background jobs choose when to retry

A job can ask to be retried after a given delay, or give up with a permanent error that ends the run as `aborted`. The attempt number now counts deliveries.

https://github.com/cs3org/reva/pull/5872
