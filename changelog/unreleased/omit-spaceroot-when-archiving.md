Bugfix: Omit spaceroot when archiving

When archiving a space there was an empty folder named `.` added. This was because of the spaceroot which was wrongly interpreted.
We now omit the space root when creating an archive.

https://github.com/cs3org/reva/pull/3999
