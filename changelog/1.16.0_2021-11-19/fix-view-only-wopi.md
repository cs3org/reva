Bugfix: Enforce permissions in public share apps

A receiver of a read-only public share could still edit files via apps like Collabora.
These changes enforce the share permissions in apps used on publicly shared resources.

https://github.com/owncloud/web/issues/5776
https://github.com/owncloud/ocis/issues/2479
https://github.com/cs3org/reva/pull/22142214
