Enhancement: Calculate etags on-the-fly for shares directory and home folder

We create references for accepted shares in the shares directory, but these
aren't updated when the original resource is modified. This PR adds the
functionality to generate the etag for the shares directory and correspondingly,
the home directory, based on the actual resources which the references point to,
enabling the sync functionality.

https://github.com/cs3org/reva/pull/1208
