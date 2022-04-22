Enhancement: Preferences driver refactor and cbox sql implementation

This PR uses the updated CS3APIs which accepts a namespace in addition to a
single string key to recognize a user preference. It also refactors the GRPC
service to support multiple drivers and adds the cbox SQL implementation.

https://github.com/cs3org/reva/pull/2696