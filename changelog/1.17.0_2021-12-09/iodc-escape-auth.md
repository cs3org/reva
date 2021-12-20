Enhancement: OIDC auth driver for ESCAPE IAM

This enhancement allows for oidc token authentication via the ESCAPE IAM service.
Authentication relies on mappings of ESCAPE IAM groups to REVA users.
For a valid token, if at the most one group from the groups claim is mapped to one REVA user, authentication can take place.

https://github.com/cs3org/reva/pull/2217
