Enhancement: make WOPI bridged apps (CodiMD) configuration non hard-coded

The configuration of the custom mimetypes has been moved to the AppProvider,
and the given mimetypes are used to configure bridged apps by sharing
the corresponding config item to the drivers.

https://github.com/cs3org/reva/pull/3401
