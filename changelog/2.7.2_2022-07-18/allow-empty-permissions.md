Bugfix: Allow empty permissions

For alias link we need the ability to set no permission on an link.
The permissions will then come from the natural permissions the receiving user
has on that file/folder

https://github.com/cs3org/reva/pull/3079
