Bugfix: upload session specific processing flag

To make every upload session have a dedicated processing status, upload sessions are now treated as in processing when all bytes have been received instead of checking the node metadata.

https://github.com/cs3org/reva/pull/4475
