Enhancement: Only load X509 on https

Currently, the EOS HTTP Client always tries to read an X509 key pair from the file system (by default, from /etc/grid-security/host{key,cert}.pem). This makes it harder to write unit tests, as these fail when this key pair is not on the file system (which is the case for the test pipeline as well).

This PR introduces a fix for this problem, by only loading the X509 key pair if the scheme of the EOS endpoint is https. Unit tests can then create a mock HTTP endpoint, which will not trigger the loading of the key pair.


https://github.com/cs3org/reva/pull/4870