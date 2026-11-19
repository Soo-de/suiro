package rpc

import (
	"context"

	"github.com/Soo-de/suiro/internal/model"
)

// Renderer turns one source's files into Kubernetes objects and returns the
// entries of the source's expectations file with them. git-collector is its
// only caller. Its input is untrusted pull request content, so the
// renderer runs each job in a subprocess with its own temporary directory
// (removed afterwards), no credentials, no network, and time and memory
// limits; plugins, exec functions and remote resources are refused.
type Renderer interface {
	// Render returns the request's objects reduced to their kept fields
	// (model.KeptFields plus model.KubeIdentityFields, see
	// model.Object.Body), in output order, with any warnings
	// (model.ReasonNamespaceMismatch). After them come the entries of the
	// expectations file at the top of Path (model.ExpectationsFile), one
	// model.KindExpectation object each, when the file exists; a file that
	// does not validate adds a model.ReasonExpectationsInvalid warning
	// instead and never fails the job. A problem with the content itself (a
	// template error, a timeout, a limit, a refused remote resource, a bad
	// path) ends the job with one input-level Problem and no objects,
	// expectation entries included: half a render is never returned.
	// The error is only for failures of the call itself, such as an
	// unreachable renderer or a cancelled ctx.
	Render(ctx context.Context, req RenderRequest) (RenderResult, error)
}

// RenderRequest is one render job. It carries the files themselves because
// the renderer has no network and no repository access.
type RenderRequest struct {
	// Source names the registration, for origins and logs.
	Source model.SourceID
	Kind   model.RenderKind
	// Path is the directory inside Files to render; the expectations file
	// is read from its top.
	Path string
	// Namespace is the destination namespace: helm's --namespace, and the
	// namespace set on namespaced objects that name none. An object naming
	// another namespace keeps it and raises model.ReasonNamespaceMismatch.
	Namespace string
	// Helm is set for model.RenderHelm.
	Helm *model.HelmSource
	// Files is the repository tree at the revision: every regular file, so
	// that kustomize bases and chart dependencies elsewhere in the
	// repository resolve. Symbolic links are left out
	// (model.ReasonSymlinkSkipped), and the .git directory is never sent.
	Files []File
}

// File is one repository file.
type File struct {
	// Path is slash-separated and relative to the repository root. The
	// renderer rejects absolute paths and paths with ".." segments.
	Path    string
	Content []byte
}

// RenderResult is the outcome of one render job.
type RenderResult struct {
	Objects  []model.Object
	Problems []model.Problem
}
