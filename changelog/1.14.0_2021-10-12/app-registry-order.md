Enhancement: Default AppProvider on top of the providers list
for each mime type

Now for each mime type, when asking for the list of mime types,
the default AppProvider, set both using the config and the 
SetDefaultProviderForMimeType method, is always in the top of the
list of AppProviders.
The config for the Providers and Mime Types for the AppRegistry changed,
using a list instead of a map. In fact the list of mime types returned by
ListSupportedMimeTypes is now ordered according the config.

https://github.com/cs3org/reva/pull/2138