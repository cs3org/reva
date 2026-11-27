Enhancement: set timeout in drives call

We now time out when trying to resolve shares and public spaces. This should 
ensure that web can at least show whatever is available in case of an outage, 
instead of hanging completely.

https://github.com/cs3org/reva/pull/6061