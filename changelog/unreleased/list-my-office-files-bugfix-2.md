Bugfix: ListMyOfficeFiles

Regex was simplified, and a cache was created to keep version folder fileinfo,
to make sure we don't need a stat for every result

https://github.com/cs3org/reva/pull/5149