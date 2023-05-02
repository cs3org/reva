Enhancement: Always list shares jail when listing spaces

Changes spaces listing to always include the shares jail, even when no shares where received.
If there are no received shares the shares jail will have the etag value `DECAFC00FEE`.

https://github.com/cs3org/reva/pull/3569
https://github.com/owncloud/ocis/issues/5190
