Bugfix: make public links work in spaces

Opening public links in spaces is currently broken. This is fixed by:
* not needing a space ID for public links
* supporting a GET directly on a public link

https://github.com/cs3org/reva/pull/5255