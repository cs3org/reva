Bugfix: Fix create version folder in EOS driver

In a read only share, a stat could fail, beacause the EOS
storage driver was not able to create the version folder
for a file in case this did not exist.
This fixes this bug impersonating the owner of the
file when creating the version folder.

https://github.com/cs3org/reva/pull/3765
