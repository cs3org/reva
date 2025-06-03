Bugfix: Return an error when EOS List errors

If we get an error while reading items, we now return the error to the user and break off the List operation
We do not want to return a partial list, because then a sync client may delete local files that are missing on the server

https://github.com/cs3org/reva/pull/5044