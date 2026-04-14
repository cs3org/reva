Enhancement: refactor EOSFS auth logic

- Use a dedicated service account for accesses made by external accounts,
  instead of impersonating the owner or using a token
- Renamed the different types of auth to be more clear (e.g. cboxAuth became systemAuth)
- Added a `InvalidAuthorization` to be returned instead of an empty auth; because empty auth maps to the system user (which is a sudo'er)

https://github.com/cs3org/reva/pull/5514
