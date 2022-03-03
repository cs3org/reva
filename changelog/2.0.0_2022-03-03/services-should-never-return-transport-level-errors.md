Bugfix: services should never return transport level errors

The CS3 API adopted the grpc error codes from the [google grpc status package](https://github.com/googleapis/googleapis/blob/master/google/rpc/status.proto). It also separates transport level errors from application level errors on purpose. This allows sending CS3 messages over protocols other than GRPC. To keep that seperation, the server side must always return `nil`, even though the code generation for go produces function signatures for rpcs with an `error` return property. That allows clients to clearly distinguish between transport level errors indicated by `err != nil` the error and application level errors by checking the status code.

https://github.com/cs3org/reva/pull/2415
