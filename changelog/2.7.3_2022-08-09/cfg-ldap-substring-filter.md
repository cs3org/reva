Enhancement: Allow to set LDAP substring filter type

We introduced new settings for the user- and groupproviders to allow
configuring the LDAP filter type for substring search. Possible values are:
"initial", "final" and "any" to do either prefix, suffix or full substring
searches.

https://github.com/cs3org/reva/pull/3087
