Bugfix: Fix an oCDAV error message

We've fixed an error message in the oCDAV service, that said "error doing GET request to data service"
even if it did a PATCH request to the data gateway. This error message is now fixed.

https://github.com/cs3org/reva/pull/3472
