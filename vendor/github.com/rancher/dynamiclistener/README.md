# [dynamiclistener](https://github.com/rancher/dynamiclistener)

DynamicListener allows you to setup a server with automatically generated (and re-generated) TLS certs with kubernetes secrets integration.

This `README` is a work in progress; aimed towards providing information for navigating the contents of this repository.

## Changing the Expiration Days for Newly Signed Certificates

By default, a newly signed certificate is set to expire 365 days (1 year) after its creation time and date.
You can use the `CATTLE_NEW_SIGNED_CERT_EXPIRATION_DAYS` environment variable to change this value.

**Please note:** the value for the aforementioned variable must be a string representing an unsigned integer corresponding to the number of days until expiration (i.e. X509 "NotAfter" value).
