// Package render is the renderer service; it implements rpc.Renderer. It
// turns Helm charts, Kustomize overlays and plain YAML into Kubernetes
// objects, and the application's expectations file (model.ExpectationsFile)
// into one object per entry, running each job in a subprocess with its own
// temporary directory, no credentials, no network, and time and memory
// limits, because its input is untrusted pull request content. Plugins and
// remote resources are disabled; a failed render becomes a reasoned error,
// not a dropped source, and so does a render that would need a file sent
// truncated (rpc.File.Truncated). An expectations file with more entries
// than the configured limit is invalid as a whole. Every object leaves
// reduced by model.ReduceBody to model.KeptFields plus
// model.KubeIdentityFields, so Secret data and environment variables never
// leave the renderer.
package render
