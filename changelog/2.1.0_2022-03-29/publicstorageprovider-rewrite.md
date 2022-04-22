Bugfix: replace public mountpoint fileid with grant fileid in ocdav

We now show the same resoucre id for resources when accessing them via a public links as when using a logged in user. This allows the web ui to start a WOPI session with the correct resource id.

https://github.com/cs3org/reva/pull/2646
https://github.com/cs3org/reva/issues/2635
