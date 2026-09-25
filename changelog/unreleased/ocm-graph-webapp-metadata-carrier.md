Enhancement: add a Graph shared-with-me response carrier that exposes
received webapp metadata at remoteItem["@ocm.webApp"] with only the appName
sibling, while preserving the existing permissions grant and array length.
Absent metadata keeps the prior DriveItem bytes unchanged; the carrier never
emits a null extension.
