Bugfix: Write upload session info atomically

We now use a lock and atomic write on upload session metadata to prevent empty reads. A virus scan event might cause the file to be truncated and then a finished event might try to read the file, just getting an empty string.

https://github.com/cs3org/reva/pull/4850
