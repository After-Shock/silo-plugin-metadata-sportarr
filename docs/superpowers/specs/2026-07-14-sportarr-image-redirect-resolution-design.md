# Sportarr Public Image Redirect Resolution

## Goal

Allow Silo to cache local Sportarr movie posters in Garage without weakening its
private-address SSRF protection.

## Design

Sportarr movie image paths remain stored as `sportarr:///api/...`. When Silo
asks the plugin to resolve an image, the plugin retains its current configured
local base URL for metadata and image-endpoint lookup, but performs a
redirect-disabled request to that local endpoint. Sportarr returns a `302`
whose `Location` is a public `https://sportarr.net/static/...` image URL. The
plugin returns that public URL to Silo; Silo's existing cache worker fetches it
and stores the result in Garage.

## Constraints

- Do not change the Sportarr metadata base URL or Silo's SSRF policy.
- Do not return or persist a Docker-private image URL to Silo's cache worker.
- Preserve existing canonical `sportarr://` storage and external HTTP URL
  passthrough.
- Resolve only the configured local Sportarr API image path. A redirect target
  is eligible only when it is absolute HTTPS, has no user info, and its host is
  public: every IP literal and every hostname DNS result must be globally
  routable. This rejects private, carrier-grade NAT, loopback, link-local,
  unspecified, multicast, reserved, and other non-global addresses. Missing,
  malformed, non-HTTPS, or non-public locations must not be substituted.

## Validation

Add failing RPC-level tests proving a local `/api/...` image resolver response
uses a redirect-disabled endpoint lookup and returns its public HTTPS Location;
test malformed/no redirect and private-host values do not produce a false URL.
Run the Docker Go suite. In production, deploy through the existing versioned,
backup-and-rollback process and requeue only the captured 92 UFC library-5
poster job IDs. Success requires every selected cache job to succeed and an
S3-backed poster path for a real Sportarr movie item.
