Bugfix: fix PROPFIND inconsistencies

Stop emitting a duplicate `oc:public-link-expiration` entry in the not-found
list, drop an erroneous fallthrough from `oc:privatelink` and skip re-adding `oc:name` 
when explicitly requested (since it's already added unconditionally) 

https://github.com/cs3org/reva/pull/5620