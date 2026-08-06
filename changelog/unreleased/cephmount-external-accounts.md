Enhancement: cephmount now supports shares to external accounts

* Grants to external accounts are now stored in a `user.reva.lwshare.<account>` xattr and are read back by ListGrants.
* Operations on their behalf run as the service account configured with `external_accounts_user_name`/`_uid`/`_gid`, cboxexternal by default, the same account the EOS driver uses.
* What they are allowed to do is decided by Reva from the stored grant rather than by the permissions of that account.
* The ResourceInfo of a shared resource now also carries a name and an etag which now is recursive, meaning you changes in subtrees also get propgated.

https://github.com/cs3org/reva/pull/5740