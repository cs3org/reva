Bugfix: shares parent reference

- change: replace `md.Id.SpaceID` with `<storage-id>?<space-id>`
- fix: parentReference
  - add space info to id
  - removes double encoding of driveId
  - new function to return relative path inside a space root
- refactor space utils:
  - reorder functions (Encode > Decode > Parse)
  - returns `SpaceID` instead of `path` in `DecodeResourceID`
  - new comments

https://github.com/cs3org/reva/pull/5189
