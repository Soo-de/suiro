package model

import "time"

// SourceID names one input source and stays the same across reads. An
// application source uses its registration's "<team>/<name>"; the AWS
// network of an environment uses "aws/<environment>", and its cluster
// "cluster/<environment>". Snapshots, reads and origins refer back to their
// registration through it.
type SourceID string

// SourceKind says which collector reads a source and which adapter
// translates its snapshots. A Terraform plan adapter would add a third kind.
type SourceKind string

const (
	// SourceGit is a registered application: Kubernetes manifests in git,
	// read by git-collector and rendered by the renderer.
	SourceGit SourceKind = "git"
	// SourceAWS is the AWS network of one environment (a VPC and its EKS
	// cluster), read live by cloud-collector.
	SourceAWS SourceKind = "aws"
	// SourceCluster is the live state of one environment's Kubernetes
	// cluster, read by cloud-collector: Namespace objects only, through a
	// ClusterRole that allows list and watch on namespaces and nothing
	// else. Its snapshots are polled and go stale like an AWS source's.
	SourceCluster SourceKind = "cluster"
)

// Source is what a collector needs to read one source. inputs derives it
// from the registration file and sends the list in Collector.Watch; the
// collector keeps it only in memory. Exactly one of Git, AWS and Cluster
// is set, matching Kind.
type Source struct {
	ID          SourceID   `json:"id"`
	Kind        SourceKind `json:"kind"`
	Environment string     `json:"environment"`
	// Owner is the registration's owner. It does not travel to collectors.
	Owner   Owner          `json:"owner"`
	Git     *GitSource     `json:"git,omitempty"`
	AWS     *AWSSource     `json:"aws,omitempty"`
	Cluster *ClusterSource `json:"cluster,omitempty"`
}

// ClusterSource locates the cluster whose namespaces cloud-collector reads.
// That is the cluster cloud-collector runs in, so it has no fields yet.
type ClusterSource struct{}

// GitSource locates an application's manifests, the way an Argo CD
// Application does: repository, path, approved revision, renderer and
// destination namespace.
type GitSource struct {
	// RepoURL is the repository's https URL, e.g. https://github.com/acme/payments.
	RepoURL string `json:"repoURL"`
	// Path is the slash-separated directory inside the repository; "" or "."
	// is the root.
	Path string `json:"path"`
	// TargetRevision is the approved branch or tag. Open pull requests
	// against it are previewed.
	TargetRevision string      `json:"targetRevision"`
	Render         RenderKind  `json:"render"`
	Helm           *HelmSource `json:"helm,omitempty"`
	// Namespace is the destination namespace. Namespaced objects that name
	// none land here.
	Namespace string `json:"namespace"`
}

// RenderKind is how the renderer turns a source's files into objects.
type RenderKind string

const (
	// RenderPlain reads every .yaml, .yml and .json file under Path,
	// recursively, in lexical order, except the ExpectationsFileName at the
	// top of Path, which is not a manifest.
	RenderPlain RenderKind = "plain"
	// RenderKustomize runs kustomize build on Path with plugins, exec
	// functions, Helm inflation and remote bases disabled.
	RenderKustomize RenderKind = "kustomize"
	// RenderHelm runs helm template on the chart at Path. Only charts and
	// dependencies inside the repository are allowed.
	RenderHelm RenderKind = "helm"
)

// HelmSource holds the helm template inputs of a RenderHelm source.
type HelmSource struct {
	// ReleaseName defaults to the application's name.
	ReleaseName string `json:"releaseName,omitempty"`
	// ValueFiles are relative to Path and applied in order.
	ValueFiles []string `json:"valueFiles,omitempty"`
	// Parameters are applied in order after ValueFiles, like --set.
	Parameters []HelmParameter `json:"parameters,omitempty"`
}

// HelmParameter is one --set (or --set-string, with ForceString) value.
type HelmParameter struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	ForceString bool   `json:"forceString,omitempty"`
}

// AWSSource locates the AWS network of one environment. Credentials are not
// part of it: cloud-collector uses its own read-only identity.
type AWSSource struct {
	Region string `json:"region"`
	VPCID  string `json:"vpcID"`
	// ClusterName is the EKS cluster whose node groups and VPC CNI add-on
	// make up the AWS-read part of the cluster profile. Empty when the
	// environment has no EKS cluster.
	ClusterName string `json:"clusterName,omitempty"`
}

// Owner is who answers for a resource: a team and, when known, one of its
// applications. The zero Owner means unowned.
type Owner struct {
	Team        string `json:"team,omitempty"`
	Application string `json:"application,omitempty"`
}

// PullRequest is an open pull request whose test merge commit is previewed.
type PullRequest struct {
	RepoURL string `json:"repoURL"`
	Number  int    `json:"number"`
	HeadSHA string `json:"headSHA"`
	// MergeSHA is GitHub's test merge commit of the head into the target
	// branch. It is empty when the pull request cannot merge; the snapshot
	// then carries ReasonPullRequestNotMergeable instead of objects.
	MergeSHA     string `json:"mergeSHA,omitempty"`
	TargetBranch string `json:"targetBranch"`
}

// ReadKey names one readable state of a source: its target branch or live
// AWS state (PullRequest 0), or the test merge commit of one pull request.
type ReadKey struct {
	Source      SourceID `json:"source"`
	PullRequest int      `json:"pullRequest,omitempty"`
}

// SourceState is a source's read state as its collector reports it. The UI
// shows it as the registration's status (connecting, connected, access
// error); there is no separate access check.
type SourceState string

const (
	// SourceConnecting means the source is registered and has no complete
	// read yet.
	SourceConnecting SourceState = "connecting"
	// SourceConnected means the last read attempt completed.
	SourceConnected SourceState = "connected"
	// SourceAccessError means the collector's identity cannot read the
	// source. Earlier complete reads stay published.
	SourceAccessError SourceState = "access-error"
	// SourceReadError means the last read failed or stopped part way
	// (throttling, network). Earlier complete reads stay published, because
	// only complete reads are ever published.
	SourceReadError SourceState = "read-error"
)

// SourceStatus is one Collector.Watch event. A collector sends it for a
// source when anything in it changes and, for every watched source, at least
// once per heartbeat interval, so a lost event is repaired by the next
// heartbeat. The server fetches a snapshot only when a digest in Reads
// differs from the one it holds.
type SourceStatus struct {
	Source SourceID    `json:"source"`
	State  SourceState `json:"state"`
	// Reason and Message explain an error State.
	Reason  ReasonCode `json:"reason,omitempty"`
	Message string     `json:"message,omitempty"`
	// LastCompleteRead is when the newest complete read finished; zero
	// before the first one.
	LastCompleteRead time.Time `json:"lastCompleteRead,omitzero"`
	// Reads lists the latest complete read of every ReadKey of the source:
	// the target branch or live state, then each open pull request. A pull
	// request that is closed or merged drops out of the list.
	Reads []ReadVersion `json:"reads"`
}

// ReadVersion announces the latest complete read of one ReadKey.
type ReadVersion struct {
	// PullRequest is nil for the target branch or live AWS state.
	PullRequest *PullRequest `json:"pullRequest,omitempty"`
	// Revision is the commit read (MergeSHA for a pull request); empty for
	// AWS.
	Revision    string    `json:"revision,omitempty"`
	Digest      Digest    `json:"digest"`
	CompletedAt time.Time `json:"completedAt"`
}
