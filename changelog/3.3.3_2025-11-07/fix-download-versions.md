Bugfix: correct content-length when downloading versions

This fix corrects a bug introduced with the implementation of range requests,
in https://github.com/cs3org/reva/pull/5367, where the content-length header was not
populated correctly when downloading versions of a file, resulting in 0b.

https://github.com/cs3org/reva/pull/5393
