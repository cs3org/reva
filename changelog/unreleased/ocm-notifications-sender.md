Enhancement: send notifications to OCM peers

Send notifications to peers that advertise support. Sending does not retry
or follow redirects, and delivery failures leave local changes intact.
Notification discovery logs only the status and length of peer responses.
