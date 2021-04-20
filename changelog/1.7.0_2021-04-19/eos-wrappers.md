Enhancement: Add wrappers for EOS and EOS Home storage drivers

For CERNBox, we need the mount ID to be configured according to the owner of a
resource. Setting this in the storageprovider means having different instances
of this service to cater to different users, which does not scale. This driver
forms a wrapper around the EOS driver and sets the mount ID according to a
configurable mapping based on the owner of the resource.

https://github.com/cs3org/reva/pull/1624
