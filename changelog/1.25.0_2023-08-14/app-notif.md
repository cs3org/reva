Enhancement: added an /app/notify endpoint for logging/tracking apps

The new endpoint serves to probe the health state of apps such as
Microsoft Office Online, and it is expected to be called by the frontend
upon successful loading of the document by the underlying app

https://github.com/cs3org/reva/pull/4044
