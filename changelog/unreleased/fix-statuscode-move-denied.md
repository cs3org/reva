Bugfix: Fixed wrong status code when moving a file to a denied path

We fixed a bug when the status code 502 was returned when moving a file to a denied path. Status code 403 (forbidden) is now returned to be compatible with oc10.

https://github.com/cs3org/reva/pull/4439
