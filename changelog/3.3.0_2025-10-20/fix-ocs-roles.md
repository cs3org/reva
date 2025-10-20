Bugfix: Remove trash and versions from OCS role

Remove trashbin and version-related permissions from conversion to OCS role, as some
space types do not support these, leading to invalid roles

https://github.com/cs3org/reva/pull/5364
