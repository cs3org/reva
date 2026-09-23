Enhancement: add safe received webapp launch

Open-in-app looks up the received share, discovers the sender, and
exchanges the shared secret for an access token on every request. Both
remote calls use the public-only OCM client, and the configured provider
domain is sent as client_id. The JSON body stays app_url and access_token.
