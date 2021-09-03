Enhancement: Add owncloudsql driver for the userprovider

We added a new backend for the userprovider that is backed by an owncloud 10 database. By default the `user_id` column is used as the reva user username and reva user opaque id. When setting `join_username=true` the reva user username is joined from the `oc_preferences` table (`appid='core' AND configkey='username'`) instead. When setting `join_ownclouduuid=true` the reva user opaqueid is joined from the `oc_preferences` table (`appid='core' AND configkey='ownclouduuid'`) instead. This allows more flexible migration strategies. It also supports a `enable_medial_search` config option when searching users that will enclose the query with `%`.

https://github.com/cs3org/reva/pull/1994
