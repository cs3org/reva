Bugfix: Avoid warning about missing .flock files

These flock files appear randomly because of file locking. We can savely ignore them.

https://github.com/cs3org/reva/pull/2645
