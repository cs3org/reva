Enhancement: log the user and user agent of each http request

The "processed http request" line now carries the authenticated user and
the request's user agent, so the usage metrics can count the users of each
client, the web UI included, whose proxy cannot see the user.

https://github.com/cs3org/reva/pull/TODO
