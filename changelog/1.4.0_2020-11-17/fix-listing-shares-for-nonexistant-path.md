Bugfix: Fix listing shares for nonexisting path

When trying to list shares for a not existing file or folder the ocs sharing implementation no longer responds with the wrong status code and broken xml.

https://github.com/cs3org/reva/pull/1316