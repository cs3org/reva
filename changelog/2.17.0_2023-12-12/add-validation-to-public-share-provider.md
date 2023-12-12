Enhancement: Add validation to the public share provider

We added validation to the public share provider. The idea behind it is that the cs3 clients will become much simpler. The provider can do the validation and return different status codes. The API clients then just need to convert CS3 status codes to http status codes.

https://github.com/cs3org/reva/pull/4372/
https://github.com/owncloud/ocis/issues/6993
