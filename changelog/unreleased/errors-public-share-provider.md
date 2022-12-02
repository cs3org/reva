Bugfix: Fix errors of public share provider according to cs3apis

All the errors returned by the public share provider
where internal errors. Now this has been fixed and the
returned errors are the one defined in the cs3apis.

https://github.com/cs3org/reva/pull/3501