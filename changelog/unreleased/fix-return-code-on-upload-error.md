Bugfix: return 409 conflict when a file was already created

We now return the correct 409 conflict status code when a file was already created by another upload.

https://github.com/cs3org/reva/pull/4872
