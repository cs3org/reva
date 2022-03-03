Bugfix: Fix spaces stat

When stating a space e.g. the Share Jail and that space contains another space, in this case a share
then the stat would sometimes get the sub space instead of the Share Jail itself.

https://github.com/cs3org/reva/pull/2501
