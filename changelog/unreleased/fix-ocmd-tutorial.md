Bugfix: Accept new userid idp format

The format for userid idp [changed](https://github.com/cs3org/cs3apis/pull/159)
and this broke [the ocmd tutorial](https://reva.link/docs/tutorials/share-tutorial/#5-1-4-create-the-share)
This PR makes the provider authorizer interceptor accept both the old and the new string format.

See https://github.com/cs3org/reva/issues/2285 and https://github.com/cs3org/reva/issues/2285
