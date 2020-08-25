Bugfix: do not restore recycle entry on purge

This PR fixes a bug in the EOSFS driver that was restoring a deleted entry
when asking for its permanent purge.
EOS does not have the functionality to purge individual files, but the whole recycle of the user.

https://github.com/cs3org/reva/pull/1099
