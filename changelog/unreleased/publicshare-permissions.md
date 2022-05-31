Bugfix: Check user permissions before updating/removing public shares

Added permission checks before updating or deleting public shares. These methods previously didn't enforce the users permissions. 

https://github.com/owncloud/ocis/issues/3498
https://github.com/cs3org/reva/pull/3900
