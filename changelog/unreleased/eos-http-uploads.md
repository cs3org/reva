Enhancement: Forward uploads to EOS via HTTP

This PR adds the functionality to forward upload requests directly to EOS
through HTTP. Any headers for chunking protocols are also forwarded, which
provides the functionality to assemble chunks in EOS itself.

https://github.com/cs3org/reva/pull/1529
