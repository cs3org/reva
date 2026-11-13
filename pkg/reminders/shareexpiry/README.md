# Share expiry reminder

The `share_expiry` kind tells users that shares expire soon. `notify` selects who is told:

- `sharer`: the initiator for a share in a home, the users and groups of the `recipient_role` role of the space for a share in a project (the initiator when the role is empty). The sharer can extend the share.
- `grantee`: the user or the e-group that the share is for, so that they know when their access ends.

A run reads the shares that expire before the furthest reminder point from the user share provider (the `expiring` invocation, through the Admin API as `service:reminders`), and resolves each share to its current path as the service user. A share whose resource is gone is skipped. The roles of a project come from the spaces registry (the `space` invocation, also as `service:reminders`), because a project only shows itself to its members.

With `days_before = [7, 1]`, a share is level 1 when it has 7 days left or less, and level 2 when it has 1 day left or less, so each recipient gets at most two mails for one expiration date. A reminder point whose window was already open when the share was last changed is skipped: a share created with 3 days left gets only the 1-day mail. A new expiration date starts again.

```toml
[serverless.services.reminders.kinds.share_expiry]
schedule = "@daily"
days_before = [7, 1]
base_url = "https://cernbox.cern.ch"
# notify = ["sharer"]            # or ["grantee"], or both
# recipient_role = "admin"
# max_shares_per_mail = 20
```

The notifications service needs a rule and a template for `share-expiry`:

```toml
[serverless.services.notifications.handlers.email.templates.share_expiry_mail]
name = "share-expiry-mail"
subject_template_path = "/etc/revad/templates/share-expiry-subject.txt"
body_template_path = "/etc/revad/templates/share-expiry.html"

[serverless.services.notifications.events.share-expiry]
type = "direct"

[serverless.services.notifications.events.share-expiry.handlers.email]
template_name = "share-expiry-mail"
```

One mail covers both sides when a recipient is sharer and grantee. The template data: `Username` (the user, or the project name), `IsProject`, `Soonest` and `SoonestText` (days left of the soonest share, e.g. `1` and `"tomorrow"`), `ShareCount` and `Plural`, `Remaining` and `HasRemaining`. Of the first `max_shares_per_mail` shares, the most urgent first:

- `Shared` and `HasShared`: the shares the recipient made: `WebURL`, `Path`, `DisplayPath`, `Name`, `File`, `Grantee`, `GranteeIsGroup`, `ExpiresAt`, `DaysLeft`, `DaysLeftText`.
- `Received` and `HasReceived`: the shares to the recipient: `WebURL` (the shared-with-me page), `Path`, `DisplayPath`, `Name`, `File`, `Sharer`, `ExpiresAt`, `DaysLeft`, `DaysLeftText`.
