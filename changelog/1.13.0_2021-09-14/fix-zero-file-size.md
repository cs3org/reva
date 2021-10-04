Bugfix: Fix uploads of empty files

This change fixes upload of empty files.
Previously this was broken and only worked for the
owncloud filesystem as it bypasses the semantics of the
InitiateFileUpload call to touch a local file.

https://github.com/cs3org/reva/pull/2055
