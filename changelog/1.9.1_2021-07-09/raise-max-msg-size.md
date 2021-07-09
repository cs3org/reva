Bugfix: Raise max grpc message size

As a workaround for listing larger folder we raised the `MaxCallRecvMsgSize` to 10MB. This should be enough for ~15k files. The proper fix is implementing ListContainerStream in the gateway, but we needed a way to test the web ui with larger collections.

https://github.com/cs3org/reva/pull/1825
