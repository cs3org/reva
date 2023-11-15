Bugfix: Fix conversion of custom ocs permissions to roles

When creating shares with just `view` permission we wrongly converted that
into the `SpacerViewer` sharing role. The correct role for that would be `legacy`.

https://github.com/cs3org/reva/pull/4342
https://github.com/owncloud/enterprise/issues/6209
