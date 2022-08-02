Enhancement: Add the reverseproxy http service

This PR adds an HTTP service which does the job of authenticating incoming
requests via the reva middleware before forwarding them to the respective
backends.  This is useful for extensions which do not have the auth mechanisms.

https://github.com/cs3org/reva/pull/2268