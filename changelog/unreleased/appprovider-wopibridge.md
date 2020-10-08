Enhancement: Resolve a WOPI bridge appProviderURL by extracting its redirect

Applications served by the WOPI bridge (CodiMD for the time being) require
an extra redirection as the WOPI bridge itself behaves like a user app.
This change returns to the client the redirected URL from the WOPI bridge,
which is the real application URL.

https://github.com/cs3org/reva/pull/1234
