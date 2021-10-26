Bugfix: Fix app open when multiple app providers are present

We've fixed the gateway behavior, that when multiple app providers are present, it always returned that we have duplicate names for app providers.
This was due the call to GetAllProviders() without any subsequent filtering by name. Now this filter mechanism is in place and the duplicate app providers error will only appear if a real duplicate is found.

https://github.com/cs3org/reva/issues/2095
https://github.com/cs3org/reva/pull/2117
