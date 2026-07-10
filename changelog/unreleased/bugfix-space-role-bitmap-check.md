Bugfix: Harden space role classification and grant checks

The `IsManager` space predicate treated any single grant-management bit as a full
manager, allowing a partial grant to act as a stealth manager. It now requires the
full add/update/remove grant triad. `IsEditor`/`IsViewer` keep classifying by their
defining capability bit so space-role variants still work. Partial grant-management
grants are rejected on space roots on both the create and update paths.

https://github.com/owncloud/reva/pull/645
