Bugfix: Fix error type in read node when file was not found 

The method ReadNode in the ocis storage didn't return the error type NotFound when a file was not found.

https://github.com/cs3org/reva/pull/1294

