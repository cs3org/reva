Bugfix: fix EOS bug in version folder creation

Due to a bug in the EOS drivers, version folders were created under
the owner of the first resource in a directory, instead of the owner
of the corresponding file. 

https://github.com/cs3org/reva/pull/5619