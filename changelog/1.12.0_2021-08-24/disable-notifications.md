Bugfix: Disable notifications

The presence of the key `notifications` in the capabilities' response would cause clients to attempt to poll the notifications endpoint, which is not yet supported. To prevent the unnecessary bandwidth we are disabling this altogether.

https://github.com/cs3org/reva/pull/1819