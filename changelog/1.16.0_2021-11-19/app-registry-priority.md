Enhancement: Add priority to app providers

Before the order of the list returned by the method FindProviders
of app providers depended from the order in which the app provider registered
themselves.
Now, it is possible to specify a priority for each app provider, and even if
an app provider re-register itself (for example after a restart), the order
is kept.

https://github.com/cs3org/reva/pull/2230
https://github.com/cs3org/cs3apis/pull/157
https://github.com/cs3org/reva/pull/2263