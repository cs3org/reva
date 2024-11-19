Bugfix: Pass the initialized logger down the stack

We now make the initialized logger available to grpc services and storage drivers, which allows for easier and more uniform logging.

https://github.com/cs3org/reva/pull/4951
