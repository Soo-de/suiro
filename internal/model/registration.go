package model

// File identifiers of the two YAML files Suiro reads from the platform
// repository. Helm turns each into a ConfigMap mounted into the server; the
// database never holds them, so every change is a reviewed pull request and
// no team can change its own registration or ownership from its own
// repository.
const (
	FileAPIVersion        = "suiro/v1"
	RegistrationsFileKind = "Registrations"
	OwnershipFileKind     = "OwnershipRules"
)

// RegistrationsFile lists the environments and the applications to analyze.
//
//	apiVersion: suiro/v1
//	kind: Registrations
//	environments:
//	  - name: prod
//	    aws: {region: eu-central-1, vpcID: vpc-0abc, clusterName: prod}
//	    cluster: {}
//	applications:
//	  - name: payments-api
//	    team: payments
//	    environment: prod
//	    source:
//	      repoURL: https://github.com/acme/payments
//	      path: deploy/chart
//	      targetRevision: main
//	      render: helm
//	      helm: {valueFiles: [values-prod.yaml]}
//	    destination: {namespace: payments}
//
// Validation, on load and in the platform repository's CI: apiVersion and
// kind as above; environment names unique; at most one environment with
// cluster set; every application's environment declared; "<team>/<name>"
// unique; team, name, repoURL, targetRevision, render and
// destination.namespace present; helm set only with render helm; paths
// relative, without "..". An invalid file is
// rejected whole and the previous one stays in force. Two applications on
// the same repoURL and path, or the same namespace, are valid but raise
// ReasonRegistrationOverlap.
type RegistrationsFile struct {
	APIVersion   string            `json:"apiVersion"`
	Kind         string            `json:"kind"`
	Environments []EnvironmentSpec `json:"environments"`
	Applications []ApplicationSpec `json:"applications"`
}

// EnvironmentSpec is one environment. Each environment is analyzed on its
// own.
type EnvironmentSpec struct {
	Name string `json:"name"`
	// AWS is the network the environment runs in; nil for an environment
	// without an AWS layer, whose results list UnexaminedAWSLayer and whose
	// workloads each have the single class ClassCluster.
	AWS *AWSSource `json:"aws,omitempty"`
	// Cluster is set on the environment of the cluster cloud-collector runs
	// in, with or without AWS: inputs then adds the source
	// "cluster/<name>", whose namespace labels the analysis uses (see
	// Namespace).
	Cluster *ClusterSource `json:"cluster,omitempty"`
}

// ApplicationSpec registers one application: a repository path deployed to
// one namespace, owned by one team. It becomes a Source with ID
// "<team>/<name>" and owner {team, name}.
type ApplicationSpec struct {
	Name        string            `json:"name"`
	Team        string            `json:"team"`
	Environment string            `json:"environment"`
	Source      ApplicationSource `json:"source"`
	Destination Destination       `json:"destination"`
}

// ApplicationSource is where the application's manifests live and how they
// render; the field names follow Argo CD's Application.
type ApplicationSource struct {
	RepoURL        string      `json:"repoURL"`
	Path           string      `json:"path"`
	TargetRevision string      `json:"targetRevision"`
	Render         RenderKind  `json:"render"`
	Helm           *HelmSource `json:"helm,omitempty"`
}

// Destination is where the application is deployed.
type Destination struct {
	Namespace string `json:"namespace"`
}

// OwnershipFile assigns owners to resources no registration produced: AWS
// objects, shared namespaces, platform add-ons.
//
//	apiVersion: suiro/v1
//	kind: OwnershipRules
//	rules:
//	  - match: "aws:*"
//	    owner: {team: platform}
//	  - match: "k8s:*/kube-system/*"
//	    owner: {team: platform}
//
// An owner resolves in this order: the registration the resource came
// from; else the first rule whose Match fits its ObjectKey; else unowned
// (ReasonUnowned). A rule that fits a registered resource and names another
// owner raises ReasonOwnershipConflict and loses. Ownership never changes a
// verdict; it decides who sees a gate's content, whom a block points to,
// and what a CI token may ask about.
type OwnershipFile struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Rules      []OwnershipRule `json:"rules"`
}

// OwnershipRule maps ObjectKeys to an owner.
type OwnershipRule struct {
	// Match is a pattern over ObjectKey in which "*" matches any run of
	// characters, "/" included; everything else matches itself.
	Match string `json:"match"`
	Owner Owner  `json:"owner"`
}

// File identifiers of the expectations file, the one Suiro file that lives
// in an application repository. Its apiVersion is FileAPIVersion.
const (
	ExpectationsFileKind = "Expectations"
	// ExpectationsFileName is the file's name at the top of the
	// registration's path.
	ExpectationsFileName = "suiro-expectations.yaml"
)

// ExpectationsFile declares an application's required paths. It lives in
// the application's own repository, as ExpectationsFileName at the top of
// the registration's path, so it changes with the code it describes. It is
// optional.
//
//	apiVersion: suiro/v1
//	kind: Expectations
//	expectations:
//	  - name: api-reaches-db
//	    sources: {workload: api}
//	    destinations: {rds: orders-db}
//	    ports: [{protocol: tcp, from: 5432, to: 5432}]
//	    sourceQuantifier: all
//	    destinationQuantifier: any
//	  - name: web-reaches-payments
//	    sources: {self: true}
//	    destinations: {service: payments/payments}
//	    ports: [{protocol: tcp, from: 80, to: 80}]
//	    sourceQuantifier: all
//	    destinationQuantifier: all
//
// Each side is written in its own platform's names, never in Suiro's
// internal keys: this application's workloads as self or workload; another
// Kubernetes workload as namespaces with labels, or as application; an AWS
// resource as rds, arn or tags; the internet as internet. One side must be
// this application's own workloads, and neither may be all, so no entry
// can demand reaching everywhere.
//
// The file travels inside its source's snapshot: the renderer reads it from
// the tree it renders and returns each entry as one KindExpectation object
// whose body is the entry, and the adapter of git snapshots turns those
// into Fragment.Expectations. An analysis therefore uses each source's
// expectations at the revision it reads that source: a pull request
// analysis takes the pull request's version for the sources of its own
// repository and the target branch's version for every other source.
//
// A destination may name a Service: the ports are then the Service's,
// which core translates for each workload behind it (see
// ExpectationStatus).
//
// Validation: the renderer rejects the whole file when it does not parse,
// has another apiVersion or kind, has more entries than the configured
// limit, or has an entry without a name or two entries with the same name.
// The adapter rejects one entry whose sources or destinations set no form;
// that uses all, endpoints or resourceKinds; whose sides both lack self and
// workload; that sets labels without namespaces; whose ports are missing or
// invalid; whose quantifiers are not "all" or "any"; or that names a
// service in its sources or beside another destination form. Either raises
// ReasonExpectationsInvalid with the entry's line, and what was rejected is
// not checked; it never fails the render. An entry whose selectors pick
// nothing is valid and becomes FindingUnmatched.
type ExpectationsFile struct {
	APIVersion   string            `json:"apiVersion"`
	Kind         string            `json:"kind"`
	Expectations []ExpectationSpec `json:"expectations"`
}

// ExpectationSpec is one required path as its owner writes it. The adapter
// turns it into an Expectation with ID "<SourceID>#<Name>", Kind
// ExpectRequired, Mode ModeWarn and the entry's Origin.
type ExpectationSpec struct {
	// Name is unique in the file.
	Name                  string           `json:"name"`
	Sources               EndpointSelector `json:"sources"`
	Destinations          EndpointSelector `json:"destinations"`
	Ports                 []PortRange      `json:"ports"`
	SourceQuantifier      Quantifier       `json:"sourceQuantifier"`
	DestinationQuantifier Quantifier       `json:"destinationQuantifier"`
}
