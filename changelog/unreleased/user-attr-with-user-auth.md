Enhancement: use user auth for updating user attrs

In EOS, we used the `cbox` acount to set user attributes, but in fact this
can be done with the user's own authorization. This is now done, so that the right
access is logged in the EOS logs.

https://github.com/cs3org/reva/pull/5420
