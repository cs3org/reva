Bugfix: correctly identify OCM embedded payloads

OCM "ro-crate" shares are to be mapped to (CS3)
EMBEDDED resource types, where "embedded" is a
generic term to signal that any JSON-represented
payload can be mapped in this way.

https://github.com/cs3org/reva/pull/5681
