Bugfix: Fix TUS uploads with transfer token only

TUS uploads had been stopped when the user JWT token expired, even if only the transfer token should be validated. Now uploads will continue as intended.

https://github.com/cs3org/reva/pull/1941