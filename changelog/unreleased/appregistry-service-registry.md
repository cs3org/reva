Enhancement: Discover app providers through the service registry

The app registry kept its own copy of the running app providers, built from a
registration each app provider pushed once, a few seconds after starting. That
copy was keyed by address, so app providers sharing one address collapsed into
a single entry and all but one became unreachable by name, and it was lost
whenever the registry restarted, since nothing ever pushed again.

App providers are ordinary reva services, so they are already in the service
registry with a reachable address and a liveness state. They now describe the
app they serve in their node metadata, and the app registry reads them from
there. An app is identified by its app name rather than by an address, it stops
being offered when its providers go away, it comes back on its own when they
return, and an app served by several nodes is load balanced across them.

As a consequence the app registry no longer has drivers: there is one
implementation, configured directly under `[grpc.services.appregistry]`, and
the `static` driver together with its `[grpc.services.appregistry.drivers.static]`
section is gone. `AddAppProvider` now returns `UNIMPLEMENTED`, and the
`app_provider_url` setting of the app provider has been removed.

https://github.com/cs3org/reva/pull/5815
