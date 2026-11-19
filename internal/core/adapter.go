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
// a policy can select pods that another source declares.
type Adapter interface {
	// Kind is the source kind whose snapshots this adapter reads.
	Kind() model.SourceKind

	// Translate turns one snapshot into a fragment. Every object ends in the
	// fragment, in model.IgnoredKinds, or as a Problem in the fragment with
	// its reason (model.UnsupportedKinds, model.ReasonKindUnrecognized, an
	// unparsable body); none is skipped silently. Translate returns an error
	// only when it cannot run at all (a canceled ctx, a snapshot of another
	// kind), never for content.
	Translate(ctx context.Context, s model.Snapshot) (model.Fragment, error)
}
