Enhancement: Allow to override default broker for go-micro base ocdav service
    
An option for setting an alternative go-micro Broker was introduced. This can
be used to avoid ocdav from spawing the (unneeded) default http Broker.

https://github.com/cs3org/reva/pull/3233
