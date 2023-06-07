Bugfix: link context in metadata client

We now disconnect the existing outgoing grpc metadata when making calls in the metadata client. To keep track of related spans we link the two contexts.

https://github.com/cs3org/reva/pull/3951
