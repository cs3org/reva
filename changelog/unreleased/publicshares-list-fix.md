Bugfix: List public shares only created by the current user

When running ocis, the public links created by a user are visible to all the
users under the 'Shared with others' tab. This PR fixes that by returning only
those links which are created by a user themselves.

https://github.com/cs3org/reva/pull/1042
