Bugfix: Fix open by default app and expose default app

We've fixed the open by default app name behaviour which previously only worked, if the default app was configured by the provider address.
We also now expose the default app on the `/app/list` endpoint to clients.

https://github.com/cs3org/reva/issues/2230
https://github.com/cs3org/cs3apis/pull/157
