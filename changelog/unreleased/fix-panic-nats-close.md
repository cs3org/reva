Bugfix: Fix panic when closing notification service

If the connection to the nats server was not yet estabished,
the service on close was panicking. This has been now fixed.

https://github.com/cs3org/reva/pull/4016
