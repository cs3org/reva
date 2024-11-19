Bugfix: enable datatx log

We now pass a properly initialized logger to the datatx implementations, allowing the tus handler to log with the same level as the rest of reva.

https://github.com/cs3org/reva/pull/4935

