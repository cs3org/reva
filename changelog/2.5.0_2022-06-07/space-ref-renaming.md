Change: Rename ocs parameter "space_ref"

We decided to deprecate the parameter "space_ref". We decided to use
"space" parameter instead. The difference is that "space" must not contain
a "path". The "path" parameter can be used in combination with "space" to
create a relative path request

https://github.com/cs3org/reva/pull/2913
