Bugfix: Nilpointer exception when owner is unknown

When the owner of a file is unknown (e.g. deleted account) the cephmount driver
had a nil pointer exception, this is now fixed.

https://github.com/cs3org/reva/pull/5992