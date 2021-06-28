Bugfix: Properly handle name collisions for deletes in the owncloud driver 

In the owncloud storage driver when we delete a file we append the deletion time to the file name.
If two fast consecutive deletes happened, the deletion time would be the same and if the two files had the same name we ended up with only one file in the trashbin.

https://github.com/cs3org/reva/pull/1833
