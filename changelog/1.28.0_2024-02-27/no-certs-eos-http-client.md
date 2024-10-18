Bugfix: no certs in EOS HTTP client

Omit HTTPS cert in EOS HTTP Client, as this causes authentication issues on EOS < 5.2.28. 
When EOS receives a certificate, it will look for this cert in the gridmap file. 
If it is not found there, the whole authn flow is aborted and the user is mapped to nobody.


https://github.com/cs3org/reva/pull/4894