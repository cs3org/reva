Enhancement: remove share first from EOS, then db

When removing shares, remove permissions from storage before going to db,
so that there can be no lingering permissions on EOS

https://github.com/cs3org/reva/pull/5484
