Enhancement: allow listing all storage spaces

To implement the drives api we allow listing all spaces by sending a spaceid `*!*`. This is a workaround until a proper filter to list all storage spaces a user has access to / can manage is implemented, or we make that the implicit default filter.

https://github.com/cs3org/reva/pull/2344
