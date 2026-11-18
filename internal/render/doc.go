// Package render is the renderer service. It turns Helm charts, Kustomize
// overlays and plain YAML into Kubernetes objects, running each job in a
// subprocess with its own temporary directory, no credentials, no network,
// and time and memory limits, because its input is untrusted pull request
// content. Plugins and remote resources are disabled; a failed render
// becomes a reasoned error, not a dropped source.
package render
