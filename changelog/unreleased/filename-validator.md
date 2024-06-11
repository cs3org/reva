Bugfix: Disallow illegal filenames filter

We have created a validator that checks if a filename is in a list of forbidden filenames.
This list is defined in the config file and can be extended by the administrator.

https://github.com/owncloud/ocis/issues/1002