Enhancement: allow from and to for trashbin in headers

Currently, from and to values for trashbin listing are passed as query parameters. With the new DAV library on the frontend, it is easier to send these as headers. Reva now accepts both, with query parameters having priority.

https://github.com/cs3org/reva/pull/5254