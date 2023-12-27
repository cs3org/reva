Bugfix: Fixed wrong status code when moving a file to a denied path

We fixed a bug when the status code was 403 instead of 502 when moving a file to a denied path to be compatible with oc10.

https://github.com/cs3org/reva/pull/4439
