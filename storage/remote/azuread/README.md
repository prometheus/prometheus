github.com/prometheus/common/azuread module
=========================================

azuread provides a http.RoundTripper that will help attach Azure AD accessToken
to the requests using the Azure AD library, which is used to retrieve accessToken
for an Azure Managed Identity.

This module is considered internal to Prometheus, without any stability
guarantees for external usage.
