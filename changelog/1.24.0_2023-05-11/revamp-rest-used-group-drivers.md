Enhancement: Revamp user/group drivers and fix user type
for lightweight accounts

* Fix the user type for lightweight accounts, using the
source field to differentiate between a primary and lw account
* Remove all the code with manual parsing of the json returned
by the CERN provider
* Introduce pagination for `GetMembers` method in the group driver
* Reduced network transfer size by requesting only needed fields for `GetMembers` method

https://github.com/cs3org/reva/pull/3821
