Enhancement: allow listing all storage spaces

To implement the drives api we now list all spaces when no filter is given. The Provider info will not contain any spaces as the client is responsible for looking up the spaces.

https://github.com/cs3org/reva/pull/2344
