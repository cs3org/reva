# gRPC Service: authsvc

To enable the service:

```
[grpc]
enabled_services = ["authsvc"]
```

Example configuration:

```
[grpc.services.authsvc]
auth_manager  = "demo"
token_manager = "jwt"
user_manager  = "demo"
```

## Directives

```
Syntax:  auth_manager = string
Default: auth_manager = "demo"
```

auth_manager specifies the auth driver to use for the authentication service.
Available drivers shipped with REVA can be consulted at the end of this section.
The default driver (demo) is a hardcoded in-memory list of well-known physicists.

```
Syntax:  token_manager = string
Default: token_manager = "demo"
```

token_manager specifies the token driver to use for the authentication service.  Available drivers shipped with REVA can be consulted at the end of this section.
The default driver (jwt) forges [JWT](https://tools.ietf.org/html/rfc7519) tokens. 

```
Syntax:  user_manager = string
Default: user_manager = "demo"
```

user_manager specifies the user manager to use for obtaining user information
like display names and groups associated to an user.
Available managers shipped with REVA can be consulted at the end of this section.
The default driver (demo) is a hardcoded in-memory catalog of well-known physicists.

## Auth managers

### Demo
The demo driver authenticates against a hardcoded in-memory catalog
of well-known physicists.
This is the list of credentials:

```
einstein => relativity
marie    => radioactivity
richard  => superfluidity 
```

### LDAP
The LDAP driver authenticates against an LDAP server.

Example configuration:

```
[grpc.services.authsvc.auth_managers.ldap"
hostname = "example.org"
port = 389
base_dn = "CN=Users,DC=example,DC=org"
filter = "(&(objectClass=person)(objectClass=user)(cn=%s))"
bind_username = "foo"
bind_password = "bar"
```

#### Directives

```
Syntax:  hostname = string
Default: hostname = ""
```

hostname specifies the hostname of the LDAP server.

```
Syntax:  port = int
Default: port = 389
```
port specifies the port of the LDAP server.

```
Syntax:  base_dn = string
Default: base_dn = ""
```

base_dn specifies the Base DN to use to query the LDAP server.

```
Syntax:  filter = string
Default: filter = ""
```
filter specifies the LDAP filter to authenticate users.
The filter needs to contains a '%s' placeholder where the username will be set
in the filter.

```
Syntax:  bind_username = string
Default: bind_username = ""
```

bind_username specifies the username to bind agains the LDAP server.

```
Syntax:  bind_password = string
Default: bind_password = ""
```

bind_password specifies the password to use to bind agains the LDAP server.

## Token managers

### JWT
The jwt manager forges [JWT](https://tools.ietf.org/html/rfc7519) tokens.

#### Directives

```
Syntax:  secret = string
Default: secret = ""
```
secret specifies the secret to use to sign a JWT token.

## User managers

### Demo
The demo manager contains a hard-coded in-memory catalog of user information
of well-known physicists. This manager is to be used with the *demo* auth manager. 
