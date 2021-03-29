Bugfix: Fix a bunch of trashbin related issues

Fixed these issues:

- Complete: Deletion time in trash bin shows a wrong date
- Complete: shared trash status code
- Partly: invalid webdav responses for unauthorized requests.
- Partly: href in trashbin PROPFIND response is wrong

Complete means there are no expected failures left.
Partly means there are some scenarios left.

https://github.com/cs3org/reva/pull/1552

