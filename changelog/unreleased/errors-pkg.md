Enhancement: New errors package

Previously, we had to manually append the name of the current package in an
error so as to get better tracing. This new errors package itself adds the name
of the package from which it was called to the error.  

https://github.com/cs3org/reva/pull/1391
