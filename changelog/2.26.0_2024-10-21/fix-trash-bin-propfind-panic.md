Bugfix: Fix trash-bin propfind panic

We fixed an issue where a trash-bin `propfind` request panicked due to a failed and therefore `nil` resource reference lookup. 

https://github.com/cs3org/reva/pull/4879
