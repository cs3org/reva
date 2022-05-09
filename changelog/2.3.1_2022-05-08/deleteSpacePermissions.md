Bugfix: Check permissions when deleting spaces

Do not allow viewers and editors to delete a space (you need to be manager)
Block deleting a space via dav service (should use graph to avoid accidental deletes)

https://github.com/cs3org/reva/pull/2827
