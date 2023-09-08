Bugfix: Fix accessing an OCM-shared resource containing spaces

Fixes the access of a resource OCM-shared containing spaces, that
previously was failing with a `NotFound` error.

https://github.com/cs3org/reva/pull/4171
