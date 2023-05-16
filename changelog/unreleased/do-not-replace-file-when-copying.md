Bugfix: Do not lose old revisions when overwriting a file during copy

We no longer delete-and-upload targets of copy operations but rather
add a new version with the source content.

This makes "overwrite when copying" behave the same as "overwrite when uploading".

Overwriting when moving a file still deletes the old file (moves it into the
trash) and replaces the whole file including the revisions of the source file.

https://github.com/cs3org/reva/pull/3896
