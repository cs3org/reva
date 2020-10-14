Enhancement: Add test user to all sites

For health monitoring of all mesh sites, we need a special user account that is present on every site. This PR adds such a user to each users-*.json file so that every site will have the same test user credentials (which are, of course, test/testpass).

This omnipresent test user allows us to perform remote checks that require authentication using the same credentials for every site, making automated check configuration much easier.

https://github.com/cs3org/reva/pull/1246
