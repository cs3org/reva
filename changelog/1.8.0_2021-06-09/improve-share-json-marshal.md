Enhancement: Improve json marshalling of share protobuf messages

Protobuf oneof fields cannot be properly handled by the native json marshaller,
and the protojson package can only handle proto messages. Previously, we were
using a workaround of storing these oneof fields separately, which made the code
inelegant. Now we marshal these messages as strings before marshalling them via
the native json package.

https://github.com/cs3org/reva/pull/1655
