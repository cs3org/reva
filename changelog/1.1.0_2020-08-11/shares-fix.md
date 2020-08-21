Bugfix: Disallow sharing the shares directory

Previously, it was possible to create public links for and share the shares
directory itself. However, when the recipient tried to accept the share, it
failed. This PR prevents the creation of such shares in the first place.

https://github.com/cs3org/reva/pull/1051
