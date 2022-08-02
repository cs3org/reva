Bugfix: Make local file connector more error tolerant

The local file connector caused Reva to throw an exception if the local file for storing site data couldn't be loaded. This PR changes this behavior so that only a warning is logged.

https://github.com/cs3org/reva/pull/1625
