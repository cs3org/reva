Bugfix: Remove malformed parameters from WOPI discovery URLs

This change fixes the parsing of WOPI discovery URLs for
MSOffice /hosting/discovery endpoint.
This endpoint is known to contain malformed 
query paramters and therefore this fix removes them.

https://github.com/cs3org/reva/pull/2051
