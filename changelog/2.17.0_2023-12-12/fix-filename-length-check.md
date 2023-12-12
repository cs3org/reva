Bugfix: Fix checking of filename length

Instead of checking for length of the filename the ocdav handler would sometimes check for complete file path.

https://github.com/cs3org/reva/pull/4302
