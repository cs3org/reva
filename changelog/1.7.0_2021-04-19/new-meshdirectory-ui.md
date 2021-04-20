Enhancement: New MeshDirectory HTTP service UI frontend with project branding

We replaced the temporary version of web frontend of the mesh directory http service with
a new redesigned & branded one. Because the new version is a more complex Vue SPA that contains image,
css and other assets, it is now served from a binary package distribution that was generated using the
[github.com/rakyll/statik](https://github.com/rakyll/statik) package. The `http.services.meshdirectory.static`
config option was obsoleted by this change.

https://github.com/cs3org/reva/issues/1502
