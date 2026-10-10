Bugfix: guard share and invite authentication records

Reject malformed share records, grantees, creators and access methods
before direct-secret authentication. Validate invitation requests,
context identities, stored records and successful repository results,
and fix the memory repository's first-insert and identity-key handling.

JSON invitation updates replace the data file before publishing the live
model; failed persistence reports an error without changing that model.

Memory invitation listings omit elapsed tokens while retaining storage
and listing of tokens without expiration. Redemption still requires a
valid expiration.

https://github.com/cs3org/reva/pull/5885
