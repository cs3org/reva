Enhancement: Add a recycle bin to cephmount

CephFS has no recycle bin, so deletes on project spaces were final. Deleted
items now go to a bin that Reva and Samba share, and users can list and
restore them from the web.

https://github.com/cs3org/reva/pull/6049