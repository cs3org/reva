Bugfix: Fix chi routing

Chi routes based on the URL.RawPath, which is not updated by the shiftPath based routing used in reva. By setting the RawPath to an empty string chi will fall pack to URL.Path, allowing it to match percent encoded path segments, e.g. when trying to match emails or multibyte characters.

https://github.com/cs3org/reva/pull/2076
