Enhancement: count warn and error log messages

Expose `reva_log_messages_total{level}` on `/metrics`
(prefix set by `logs.namespace`),
incremented for each emitted log message at warn level or above.

https://github.com/cs3org/reva/pull/6018
