Enhancement: Add auth protocol based on user agent

Previously, all available credential challenges are given to the client, 
for example, basic auth, bearer token, etc ...
Different clients have different priorities to use one method or another, 
and before it was not possible to force a client to use one method without 
having a side effect on other clients.

This PR adds the functionality to target a specific auth protocol based
on the user agent HTTP header.

https://github.com/cs3org/reva/pull/1350
