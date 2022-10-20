Enhancement: added support for configuring language locales in apps

This is a partial backport from edge: we introduce a language option
in the appprovider, which if set is passed as appropriate parameter
to the external apps in order to force a given localization. In particular,
for Microsoft Office 365 the DC_LLCC option is set as well.
The default behavior is unset, where apps try and resolve the
localization from the browser headers.

https://github.com/cs3org/reva/pull/3303
