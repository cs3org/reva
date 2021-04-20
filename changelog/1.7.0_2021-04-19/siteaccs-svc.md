Enhancement: Site Accounts service for API keys

This update adds a new service to Reva that handles site accounts creation and management. Registered sites can be assigned an API key through a simple web interface which is also part of this service. This API key can then be used to identify a user and his/her associated (vendor or partner) site.

Furthermore, Mentix was extended to make use of this new service. This way, all sites now have a stable and unique site ID that not only avoids ID collisions but also introduces a new layer of security (i.e., sites can only be modified or removed using the correct API key). 

https://github.com/cs3org/reva/pull/1506
