Bugfix: fixed parsing of OCM discovery payload

The principle is that we should not fail the JSON parsing
on unsupported capabilities, but only on out of spec payloads.
Therefore, `ResourceType` must be a generic string, to be
validated afterwards.

https://github.com/cs3org/reva/pull/5676
