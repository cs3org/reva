Enhancement: Add missing image MIME types (AVIF, JPEG XL, CR3, RW2) and fix BMP

Added MIME type mappings for modern image formats that were missing from the
hardcoded MIME type map: `.avif` (image/avif), `.jxl` (image/jxl),
`.cr3` (image/x-canon-cr3), and `.rw2` (image/x-panasonic-rw2).

Also updated `.bmp` from the legacy `image/x-ms-bmp` to the IANA standard
`image/bmp` (RFC 7903).

Without these entries, files with these extensions were detected as
`application/octet-stream` by the storage provider, causing them to be
invisible to KQL queries like `Mediatype:image/*` in the search index.

Discovered by uploading an AVIF file to oCIS and observing that
`Mediatype:image/*` did not return it, while a filename search did. The root
cause is that reva's `mime.Detect()` uses a hardcoded map and does not fall
back to the OS mime.types database.

https://github.com/owncloud/reva/pull/570
