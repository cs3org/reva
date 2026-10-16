# Link expiry reminder

The `link_expiry` kind tells the holders of public links that their links
expire soon: the initiator for a link in a home, the administrators' e-group
for a link in a project space.

A run reads the links that expire before the furthest reminder point from the
public share provider (the `expiring` invocation, through the Admin API as
`service:reminders`), and resolves each link to its current path as the
service user. A link whose resource is gone is skipped.

With `days_before = [7, 1]`, a link is level 1 when it has 7 days left or
less, and level 2 when it has 1 day left or less, so its holder gets at most
two mails for one expiration date. A reminder point whose window was already
open when the link was last changed is skipped: a link created with 3 days
left gets only the 1-day mail. A new expiration date starts again.

```toml
[serverless.services.reminders.kinds.link_expiry]
schedule = "@daily"
days_before = [7, 1]
base_url = "https://cernbox.cern.ch"
# public_link_base_url = "https://cernbox.cern.ch/s/"
# project_prefix = "/eos/project/"
# project_admins_group = "cernbox-project-%s-admins"
# max_links_per_mail = 20
```

The notifications service needs a rule and a template for `link-expiry`:

```toml
[serverless.services.notifications.handlers.email.templates.link_expiry_mail]
name = "link-expiry-mail"
subject_template_path = "/etc/revad/templates/link-expiry-subject.txt"
body_template_path = "/etc/revad/templates/link-expiry.html"

[serverless.services.notifications.events.link-expiry]
type = "direct"

[serverless.services.notifications.events.link-expiry.handlers.email]
template_name = "link-expiry-mail"
```

The template data: `Username` (the user, or the project name), `IsProject`,
`Soonest` and `SoonestText` (days left of the soonest link, e.g. `1` and
`"tomorrow"`), `LinkCount` and `Plural`, `Links` (the first
`max_links_per_mail`, the most urgent first: `Href`, `WebURL`, `Path`,
`DisplayPath`, `Name`, `Icon`, `ExpiresAt`, `DaysLeft`, `DaysLeftText`),
`Remaining` and `HasRemaining`.
