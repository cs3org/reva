Enhancement: Support multiple issuer in OIDC auth driver.

The OIDC auth driver supports now multiple issuers. Users of
external providers are then mapped to a local user by a 
mapping files. Only the main issuer (defined in the config
with `issuer`) and the ones defined in the mapping are
allowed for the verification of the OIDC token.

https://github.com/cs3org/reva/pull/3839