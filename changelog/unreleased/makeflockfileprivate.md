Enhancement: Make the function flockFile private.

Having that function exported is tempting people to use the func
to get the name for calling the lock functions. That is wrong, as
this function is just a helper to generate the lock file name from
a given file to lock.

https://github.com/cs3org/reva/pull/3195
