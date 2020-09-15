Bugfix: up and download of file shares

The shared folder logic in the gateway storageprovider was not allowing file up and downloads for single file shares. We now check if the reference is actually a file to determine if up / download should be allowed.

https://github.com/cs3org/reva/pull/1170
