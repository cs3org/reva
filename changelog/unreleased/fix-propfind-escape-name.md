Bugfix: Properly escape oc:name in propfind response

The oc:name property in the ocdav propfind response might contain
XML special characters. We now apply the proper escaping on that
property.

https://github.com/cs3org/reva/pull/3255
https://github.com/owncloud/ocis/issues/4474
