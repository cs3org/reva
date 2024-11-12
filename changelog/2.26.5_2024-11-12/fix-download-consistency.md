Bugfix: make etag always match content on downloads

We added an openReaderfunc to the Download interface to give drivers a way to guarantee that the reader matches the etag returned in a previous GetMD call.

https://github.com/cs3org/reva/pull/4926
https://github.com/cs3org/reva/pull/4923

