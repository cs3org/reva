Enhancement: Add hide flag to shares

We have added the ability to hide shares through the
ocs/v2.php/apps/files_sharing/api/v1/shares/pending/ endpoint
by appending a POST-Variable called hide which can be true or false.

https://github.com/cs3org/reva/pull/4194/files