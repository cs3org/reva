Enhancement: Add project status filtering to ListStorageSpaces

Projects can now be filtered by status when listing storage spaces by passing 
the project status in the request's Opaque map. If no status is 
specified, the default behavior lists only active projects. A new helper function `WithProjectStatus()` has been added to 
facilitate setting the status parameter in list requests.

https://github.com/cs3org/reva/pull/5545
