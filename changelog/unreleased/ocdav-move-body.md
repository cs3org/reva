Enhancement: Explicitly return on ocdav move requests with body

Added a check if a ocdav move request contains a body. If it does a 415 415 (Unsupported Media Type) will be returned.

https://github.com/owncloud/ocis/issues/3882
https://github.com/cs3org/reva/pull/2974
