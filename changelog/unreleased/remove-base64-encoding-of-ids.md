Change: Do not encode webDAV ids to base64

We removed the encoding of the IDs and use the format <storageID>!<opaqueID> with a delimiter. The used delimiter is url safe anc belongs to the reserved keys.

https://github.com/cs3org/reva/pull/2542
