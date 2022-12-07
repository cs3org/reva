Enhancement: Make Refresh Lock operation WOPI compliant

We now support the WOPI compliant `UnlockAndRelock` operation. This has been implemented in the DecomposedFS. To make use of it, we need a compatible WOPI server.

https://github.com/cs3org/reva/pull/3286
https://learn.microsoft.com/en-us/microsoft-365/cloud-storage-partner-program/rest/files/unlockandrelock
