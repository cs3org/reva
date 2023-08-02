Enhancement: Handle target in OpenInApp response

This PR adds the OpenInApp.target and AppProviderInfo.action properties
to the respective responses (/app/open and /app/list), to support
different app integrations.
In addition, the archiver was extended to use the name of the file/folder
as opposed to "download", and to include a query parameter to
override the archive type, as it will be used in an upcoming app.

https://github.com/cs3org/reva/pull/4077
