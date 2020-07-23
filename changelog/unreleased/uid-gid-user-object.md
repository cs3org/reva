Enhancement: Add UID and GID to the user object from user package

Currently, the UID and GID for users need to be read from the local system which
requires local users to be present. This change retrieves that information from
the user and auth packages and adds methods to retrieve it.

https://github.com/cs3org/reva/pull/995
