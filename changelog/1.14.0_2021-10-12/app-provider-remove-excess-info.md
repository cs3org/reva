Bugfix: Remove excess info from the http list app providers endpoint

We've removed excess info from the http list app providers endpoint.
The app provider section contained all mime types supported by a certain app provider, 
which led to a very big JSON payload and since they are not used they have been removed again.
Mime types not on the mime type configuration list always had `application/octet-stream` as a file extension and `APPLICATION/OCTET-STREAM file` as name and description. Now these information are just omitted.

https://github.com/cs3org/reva/pull/2149
https://github.com/owncloud/ocis/pull/2603
https://github.com/cs3org/reva/pull/2138
