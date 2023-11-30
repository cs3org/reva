Enhancement: Handle trashbin file listings concurrently

We now use a concurrent walker to list files in the trashbin. This
improves performance when listing files in the trashbin.

https://github.com/cs3org/reva/pull/4377
https://github.com/cs3org/reva/pull/4374
https://github.com/owncloud/ocis/issues/7844