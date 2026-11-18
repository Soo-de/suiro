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

// RegistrationsFile lists the environments and the applications to analyse.
//
//	apiVersion: suiro/v1
//	kind: Registrations
//	environments:
//	  - name: prod
//	    aws: {region: eu-central-1, vpcID: vpc-0abc, clusterName: prod}
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
// kind as above; environment names unique; every application's
// environment declared; "<team>/<name>" unique; team, name, repoURL,
// targetRevision, render and destination.namespace present; helm set only
// with render helm; paths relative, without "..". An invalid file is
// rejected whole and the previous one stays in force. Two applications on
// the same repoURL and path, or the same namespace, are valid but raise
// ReasonRegistrationOverlap.
type RegistrationsFile struct {
	APIVersion   string            `json:"apiVersion"`
	Kind         string            `json:"kind"`
	Environments []EnvironmentSpec `json:"environments"`
	Applications []ApplicationSpec `json:"applications"`
}

// EnvironmentSpec is one environment. Each environment is analysed on its
// own; the MVP runs one.
type EnvironmentSpec struct {
	Name string `json:"name"`
	// AWS is the network the environment runs in; nil for an environment
	// without an AWS layer, whose results list UnexaminedAWSLayer.
	AWS *AWSSource `json:"aws,omitempty"`
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
