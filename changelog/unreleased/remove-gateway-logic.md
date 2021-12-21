Enhancement: Remove gateway logic

Removed logic in gateway that cares about absolute paths. From now on clients are responsible for converting
pathes to id based requests.

https://github.com/cs3org/reva/pull/2394
