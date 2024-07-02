Bugfix: Disallow reserved filenames

We now disallow the reserved `..` and `.` filenames. They are only allowed as destinations of move or copy operations.

https://github.com/cs3org/reva/pull/4740
