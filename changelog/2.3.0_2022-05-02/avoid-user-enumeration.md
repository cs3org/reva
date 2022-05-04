Change: avoid user enumeration

sending PROPFIND requests to `../files/admin` did return a different response than sending the
same request to `../files/notexists`. This allowed enumerating users.
This response was changed to be the same always

https://github.com/cs3org/reva/pull/2735
