Bugfix: Fix the returned permissions for webdav uploads

We've fixed the returned permissions for webdav uploads. It did not consider 
shares and public links for the permission calculation, but does so now.

https://github.com/cs3org/reva/pull/2179
https://github.com/cs3org/reva/pull/2151
