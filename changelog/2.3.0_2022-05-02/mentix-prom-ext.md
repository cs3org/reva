Enhancement: Mentix PromSD extensions

The Mentix Prometheus SD scrape targets are now split into one file per service type, making health checks configuration easier. Furthermore, the local file connector for mesh data and the site registration endpoint have been dropped, as they aren't needed anymore.

https://github.com/cs3org/reva/pull/2560
