Enhancement: add shared OCM address parsing

Add shared parsers for OCM addresses and server hosts. Identifiers retain
their opaque UTF-8 bytes, and server comparison preserves the complete
host and optional port spelling after lowercasing. Malformed input is
rejected without network access.
