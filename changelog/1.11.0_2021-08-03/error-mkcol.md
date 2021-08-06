Bugfix: Error when creating folder with existing name

When a user tried to create a folder with the name of an existing file or folder the service didn't return a response body containing the error.

https://github.com/cs3org/reva/pull/1907
