Bugfix: Override provider if was previously registered

Previously if an AppProvider registered himself two times, for example
after a failure, the mime types supported by the provider contained
multiple times the same provider.
Now this has been fixed, overriding the previous one.

https://github.com/cs3org/reva/pull/2168
