Enhancement: make less stat requests

The /dav/spaces endpoint now constructs a reference instead of making a lookup grpc call, reducing the number of requests. 

https://github.com/cs3org/reva/pull/3000
