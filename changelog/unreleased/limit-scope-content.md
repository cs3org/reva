Enhancement: Limit publicshare and resourceinfo scope content 

We changed the publicshare and resourceinfo scopes to contain only necessary values.
This reduces the size of the resulting token and also limits the amount of data which can be leaked.

https://github.com/owncloud/ocis/issues/2479
https://github.com/cs3org/reva/pull/2093
