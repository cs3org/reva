Bugfix: guard share and invite authentication records

Reject malformed share records, grantees, creators and access methods
before direct-secret authentication. Validate invitation requests,
context identities, stored records and successful repository results,
and fix the memory repository's first-insert and identity-key handling.

https://github.com/cs3org/reva/pull/5885
