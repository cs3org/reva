Enhancement: Rework LDAP configuration of user and group providers

We reworked to LDAP configuration of the LDAP user and group provider to
share a common configuratio scheme. Additionally the LDAP configuration
no longer relies on templating LDAP filters in the configuration which
is error prone and can be confusing. The new LDAP configuration basically looks
this

```ini
[grpc.services.userprovider.drivers.ldap]
uri="ldaps://localhost:636"
insecure=true
user_base_dn="ou=testusers,dc=owncloud,dc=com"
group_base_dn="ou=testgroups,dc=owncloud,dc=com"
user_filter=""
user_objectclass="posixAccount"
group_filter=""
group_objectclass="posixGroup"
bind_username="cn=admin,dc=owncloud,dc=com"
bind_password="admin"
idp="http://localhost:20080"

[grpc.services.userprovider.drivers.ldap.user_schema]
id="entryuuid"
displayName="displayName"
userName="cn"

[grpc.services.userprovider.drivers.ldap.group_schema]
id="entryuuid"
displayName="cn"
groupName="cn"
member="memberUID"
```

`uri` defines the LDAP URI of the destination Server

`insecure` allows to disable TLS Certifictate Validation (for development setups)

`user_base_dn`/`group_base_dn` define the search bases for users and groups

`user_filter`/`group_filter` allow to define additional LDAP filter of users and groups.
This could be e.g. `(objectclass=owncloud)` to match for an additional objectclass.

`user_objectclass`/`group_objectclass` define the main objectclass of Users and Groups.
These are used to construct the LDAP filters

`bind_username`/`bind_password` contain the authentication information for the LDAP connections

The `user_schema` and `group_schema` sections define the mapping from CS3
user/group attributes to LDAP Attributes

https://github.com/cs3org/reva/pull/2708
https://github.com/cs3org/reva/issues/2122
