Bugfix: do not report leading zeros in adler32 checksum

The owncloud sync client does an exact comparision, and it
does not expect a leading zero, leading it to reject the file

https://github.com/cs3org/reva/pull/5988