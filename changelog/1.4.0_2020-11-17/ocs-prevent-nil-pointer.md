Bugfix: prevent nil pointer when listing shares

We now handle cases where the grpc connection failed correctly by no longer trying to access the response status.

https://github.com/cs3org/reva/pull/1317