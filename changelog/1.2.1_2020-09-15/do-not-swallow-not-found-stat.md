Bugfix: Do not swallow 'not found' errors in Stat

Webdav needs to determine if a file exists to return 204 or 201 response codes. When stating a non existing resource the NOT_FOUND code was replaced with an INTERNAL error code. This PR passes on a NOT_FOUND status code in the gateway.

https://github.com/cs3org/reva/pull/1124
