Bugfix: do not use version folders for xattrs in EOS

This was a workaround needed some time ago. We revert now
to the standard behavior, xattrs are stored on the files.

https://github.com/cs3org/reva/pull/4520
