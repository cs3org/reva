Enhancement: Be defensive about wrongly quoted etags

When ocdav renders etags it will now try to correct them to the definition as *quoted strings* which do not contain `"`. This prevents double or triple quoted etags on the webdav api.

https://github.com/cs3org/reva/pull/1870
