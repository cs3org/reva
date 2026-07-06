Bugfix: Prevent setting an expiration on the last space manager

Setting an expiration date on the last permanent manager of a space was allowed.
Once that grant expired the space would be left without any manager. The check
that ensures at least one permanent manager remains was only run when a grant
dropped the manager role; it now also runs when a manager grant is given an
expiration, since an expiring manager is not a permanent manager.

https://github.com/owncloud/reva/pull/631
https://github.com/owncloud/ocis/issues/11205
