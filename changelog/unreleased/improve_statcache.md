Bugfix: Add ArbitraryMetadataKeys to statcache key

Otherwise stating with and without them would return the same result (because it is cached)

https://github.com/cs3org/reva/pull/2440
