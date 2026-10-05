Change: [OCISDEV-1433] Replace GRPC_MAX_CONNECTION_AGE with client keepalive

The grpc clients can now send a keepalive ping while a request is in flight and
fail the requests on a connection whose peer stops answering, instead of waiting
for as long as the caller allows. Set GRPC_CLIENT_KEEPALIVE_TIME to a duration
such as '20s' to enable this; GRPC_CLIENT_KEEPALIVE_TIMEOUT is how long the
answer to a ping is waited for and defaults to '10s'. Leaving the keepalive time
unset sends no pings at all, which is grpc's own behavior.

GRPC_MAX_CONNECTION_AGE has been removed. It only closed healthy connections on
a timer, never ended a request that was already in flight, and silently did
nothing when its value had no unit suffix.

https://github.com/owncloud/reva/pull/758
