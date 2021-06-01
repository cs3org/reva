Enhancement: Add support for lightweight user types

This PR adds support for assigning and consuming user type when setting/reading
users. These changes are further required to enable setting varying access
scopes for different types of users, such as lightweight accounts which can only
access resources shared with them.

https://github.com/cs3org/reva/pull/1744
https://github.com/cs3org/cs3apis/pull/120
