# Suiro

Suiro (水路, "waterway") tells you, before a change is merged, whether it breaks a network path that must work or
opens one that must stay closed. It reads Kubernetes manifests from git and the AWS network read-only, builds one
reachability graph across both layers, and gives every path a verdict: allow, deny, placement-dependent or unknown,
with the gates it looked at and the reason.

Status: early development.

## Build

Requires Go (version in `go.mod`).

```sh
make build      # all programs into bin/
make test
make help       # every target
```
