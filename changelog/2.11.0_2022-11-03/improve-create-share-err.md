Enhancement: Improve CreateShare grpc error reporting

The errorcode returned by the share provider when creating a share where the sharee
is already the owner of the shared target is a bit more explicit now. Also debug logging
was added for this.

https://github.com/cs3org/reva/pull/3223
