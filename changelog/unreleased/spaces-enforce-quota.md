Enhancement: Enforce quota on Storage Spaces

Spaces contain a quota property. Previous quota lookups are relative to the home or root node of a storage. These changes take into consideration uploading to a location that is under a storage space.

https://github.com/cs3org/reva/pull/2164