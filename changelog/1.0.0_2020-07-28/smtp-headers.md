Enhancement: Add required headers to SMTP client to prevent being tagged as spam

Mails being sent through the client, specially through unauthenticated SMTP were
being tagged as spam due to missing headers.

https://github.com/cs3org/reva/pull/970
