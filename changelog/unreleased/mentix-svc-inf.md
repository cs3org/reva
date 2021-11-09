Enhancement: Mentix service inference

Previously, 4 different services per site had to be created in the GOCDB. This PR removes this redundancy by infering all endpoints from a single service entity, making site administration a lot easier.
 
https://github.com/cs3org/reva/pull/2251
