Enhancement: expose persisted received webapp app names on shared-with-me
responses at remoteItem["@ocm.webApp"] only when a share carries exactly one
webapp protocol; absent or ambiguous persisted protocols leave the extension
out so duplicate or mismatched data is not surfaced.
