Bugfix: Share jail now works properly when accessed as a space

When accessing shares via the virtual share jail we now build correct relative references before forwarding the requests to the correct storage provider.

https://github.com/cs3org/reva/pull/2904