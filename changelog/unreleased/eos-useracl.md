Enhancement: do not read eos user ACLs any longer

This PR drops the compatibility code to read eos user ACLs
in the eos binary client, and aligns it to the GRPC client.

https://github.com/cs3org/reva/pull/4892
