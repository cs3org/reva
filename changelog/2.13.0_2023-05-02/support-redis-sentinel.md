Enhancement: Add support for redis sentinel caches

We added support for redis sentinel. The sentinel configuration is given in
the cache node in the following form:
    
    <host>/<name of master>
    
e.g.
    
    10.10.0.207/mymaster

https://github.com/cs3org/reva/pull/3697
https://github.com/owncloud/ocis/issues/5645
