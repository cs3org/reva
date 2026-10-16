Change: remove the ScienceMesh open-in-app route

`POST /sciencemesh/open-in-app` is removed. It returned the exchanged access
token to the browser as JSON. Received webapp shares now launch only through
`POST /sciencemesh/webapp-launch`, which posts the token straight to the
validated app URL. The lightweight account scope no longer allows the old
path. The CS3 app provider `OpenInApp` call and the `reva open-in-app` CLI
command are unchanged.
