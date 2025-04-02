Bugfix: fix broken handling of range requests

Currently, the video preview in public links is broken, because the browser sends a "Range: bytes=0-" request. Since EOS over gRPC returns a ReadCloser on the file, which is not seekable, Reva currently returns a 416 RequestedRangeNotSatisfiable response, breaking the video preview.

This PR modifies this behaviour to ignore the Range request in such cases.

Additionally, some errors where removed. For example, when the request does not contain bytes=, Reva currently returns an error. However, RFC 7233 states:

> An origin server MUST ignore a Range header field that contains a range unit it does not understand

Thus, we now ignore these requests instead of returning a 416.

https://github.com/cs3org/reva/pull/5133