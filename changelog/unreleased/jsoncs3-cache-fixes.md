Bugfix: jsoncs3 cache fixes

The jsoncs3 share manager now retries persisting if the file already existed and picks up the etag of the upload response in all cases.

https://github.com/cs3org/reva/pull/4968
https://github.com/cs3org/reva/pull/4532
