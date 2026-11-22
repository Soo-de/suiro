package k8s

import "github.com/Soo-de/suiro/internal/core"

// New returns the adapter for model.SourceGit snapshots: rendered
// Kubernetes objects and expectation entries.
func New() core.Adapter {
	panic("k8s: New is not implemented")
}

// NewCluster returns the adapter for model.SourceCluster snapshots, which
// hold Namespace objects only.
func NewCluster() core.Adapter {
	panic("k8s: NewCluster is not implemented")
}
