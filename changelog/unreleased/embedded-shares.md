Enhancement: OCM Embedded shares

This PR introduces OCM embedded shares

* Adds functionality to store embedded shares (where the shared data is embedded in the OCM share payload)
* Adds filters to `ListReceivedOCMShares` call and adapts to the new fields `SharedResourceType` and `RecipientType`
* Adds an endpoint to list embedded shares (using the previously mentioned filters)

https://github.com/cs3org/reva/pull/5452