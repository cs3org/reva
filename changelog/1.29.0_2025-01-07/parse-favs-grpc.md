Bugfix: handle parsing of favs over gRPC

To store user favorites, the key `user.http://owncloud.org/ns/favorite` maps to a list of users, in the format `u:username=1`. Right now, extracting the "correct" user doesn't happen in gRPC, while it is implemented in the EOS binary client. This feature has now been moved to the higher-level call in eosfs.

https://github.com/cs3org/reva/pull/4973