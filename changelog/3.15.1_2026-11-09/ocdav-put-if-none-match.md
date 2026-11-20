Enhancement: Support If-None-Match on PUT

A PUT with `If-None-Match: *` now gets a 412 when the file exists, so a client
that creates a new file does not overwrite a file created in the meantime.

https://github.com/cs3org/reva/pull/5989
