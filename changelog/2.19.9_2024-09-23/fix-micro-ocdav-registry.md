Bugfix: Fix micro ocdav service init and registration

We no longer call Init to configure default options because it was replacing the existing options.

https://github.com/cs3org/reva/pull/4842
https://github.com/cs3org/reva/pull/4774
