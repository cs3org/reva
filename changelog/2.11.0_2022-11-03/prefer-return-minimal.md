Enhancement: support Prefer: return=minimal in PROPFIND

To reduce HTTP body size when listing folders we implemented https://datatracker.ietf.org/doc/html/rfc8144#section-2.1 to omit the 404 propstat part when a `Prefer: return=minimal` header is present.

https://github.com/cs3org/reva/pull/3222
