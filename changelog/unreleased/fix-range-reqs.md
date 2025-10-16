Enhancement: Proper support of Range requests

Up to now, Reva supported Range requests only when the requested content
was copied into Reva's memory. This has now been improved: the ranges can
be propagated upto the storage provider, which only returns the requested
content, instead of first copying everything into memory and then only
returning the requested ranges

https://github.com/cs3org/reva/pull/5367
