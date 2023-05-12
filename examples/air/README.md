# Air config file

Put this config file in the root of the project and start the Reva daemon by
running [Air](https://github.com/cosmtrek/air) to enable live-reload for it.

This config follows the setup used at CERN which leverages [Supervisord](http://supervisord.org/)
for controlling the services running. It is easy to make changes to use the more
common [systemd.service].

To do this, just replace the following two lines in the file:

```
bin = "/usr/bin/systemctl start revad"
cmd = "make revad && /usr/bin/systemctl stop revad && cp cmd/revad/revad /usr/bin/revad"
```
