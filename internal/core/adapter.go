package core

import (
	"context"

	"github.com/Soo-de/suiro/internal/model"
)

// Adapter translates the snapshots of one source format into the model.
// There is one per format (rendered Kubernetes YAML in core/adapter/k8s, AWS
// API objects in core/adapter/aws, later Terraform plan JSON), each
// registered by its Kind and each passing the internal/conformance suite.
//
// An adapter carries no meaning of its own: it restates rules as gates whose
// FilterSemantics describe how they decide (an isolating, additive, stateful
// NetworkPolicy; a stateful Security Group; an ordered, stateless Network
// ACL) and leaves selectors unresolved. Core applies the semantics, because
// a policy can select pods of another source's workloads. What it does
// restate follows the platform: a default the platform's API fixes is
// written out (an omitted protocol becomes TCP), an unwritten field that
// means "every" becomes the explicit form (model.Peer.All, model.AllPorts),
// and an element outside the model is dropped as model.Protocol describes.
// It raises only the reason codes whose model.Reasons row names
// model.ProducerAdapter.
type Adapter interface {
	// Kind is the source kind whose snapshots this adapter reads.
	Kind() model.SourceKind

	// Translate turns one snapshot into a fragment that passes
	// model.Fragment.Validate. Every object ends in the fragment, in
	// model.IgnoredKinds, or as a Problem in the fragment with its reason
	// (model.UnsupportedKinds, model.ReasonKindUnrecognized,
	// model.ReasonWorkloadKindUnsupported, model.ReasonObjectInvalid for a
	// body that does not parse or that the platform would reject); none is
	// skipped silently. Translate returns an error only when it cannot run
	// at all (a canceled ctx, a snapshot of another kind), never for
	// content.
	Translate(ctx context.Context, s model.Snapshot) (model.Fragment, error)
}
