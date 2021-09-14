Bugfix: Do not truncate logs on restart

This change fixes the way log files were opened.
Before they were truncated and now the log file
will be open in append mode and created it if it
does not exist.

https://github.com/cs3org/reva/pull/2047
