Bugfix: Fixes the DefaultQuotaBytes in EOS

We were setting the default logical quota to 1T, resulting 
on only 500GB available to the user.

https://github.com/cs3org/reva/pull/3492
