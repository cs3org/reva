Enhancement: do not use root on EOS

Currently, the EOS drivers use root authentication for many different operations. This has now been changed to use one of the following:
* cbox, which is a sudo'er
* daemon, for read-only operations
* the user himselft

Note that home creation is excluded here as this will be tackled in a different PR.

https://github.com/cs3org/reva/pull/4977/