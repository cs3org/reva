Bugfix: Do not panic on remote.php/dav/files/

Currently requests to /remote.php/dav/files/ result in panics since we cannot longer strip the user + destination from the url. This fixes the server response code and adds an error body to the response.

https://github.com/cs3org/reva/pull/1320