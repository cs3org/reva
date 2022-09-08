Enhancement: Improve tokeninfo endpoint

We added more information to the tokeninfo endpoint. `aliaslink` is a bool value indicating if the permissions are 0.
`id` is the full id of the file. Both are available to all users having the link token. `spaceType` (indicating the space type) 
is only available if the user has native access

https://github.com/cs3org/reva/pull/3179
