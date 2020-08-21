Bugfix: Ensure ignoring stray shares

When using the json shares manager, it can be the case we found a share with a resource_id that no longer exists. This PR aims to address his case by changing the contract of getPath and return the result of the STAT call instead of a generic error, so we can check the cause.

https://github.com/cs3org/reva/pull/1064 