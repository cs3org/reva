Bugfix: Fix Makefile error on Ubuntu

I've fixed Makefile using sh which is defaulted to dash in ubuntu, dash doesn't support
`[[ ... ]]` syntax and Makefile would throw `/bin/sh: 1: [[: not found` errors.

https://github.com/cs3org/reva/issues/3773
https://github.com/cs3org/reva/pull/3780
