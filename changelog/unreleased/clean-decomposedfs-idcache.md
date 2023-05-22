Bugfix: Clean IDCache properly

decomposedfs' subpackage `tree` uses an idCache to avoid reading too often from disc. In case of a `move` or `delete` this cache was
properly cleaned, but when renaming a file (= move with same parent) the cache wasn't cleaned. This lead to strange behaviour when
uploading files with the same name and renaming them

https://github.com/cs3org/reva/pull/3903
https://github.com/cs3org/reva/pull/3910
