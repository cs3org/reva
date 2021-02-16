Enhancement: Indicate in EOS containers that TUS is not supported

The OCDAV propfind response previously hardcoded the TUS headers due to which
clients such as phoenix used the TUS protocol for uploads, which EOS doesn't
support. Now we pass this property as an opaque entry in the containers
metadata.

https://github.com/cs3org/reva/pull/1415
