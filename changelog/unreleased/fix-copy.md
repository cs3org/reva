Bugfix: make COPY requests work again

COPY requests were broken, because during the upload-part of the copy, no Content-Length header was set.

https://github.com/cs3org/reva/pull/5264