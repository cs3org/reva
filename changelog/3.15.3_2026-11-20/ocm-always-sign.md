Change: OCM always signs and needs signing_key and endpoint

We will now enforce that all our outgoing OCM requests are signed
and this also means that a signing key and the public endpoint are
required if OCM is enabled.


https://github.com/cs3org/reva/pull/6031