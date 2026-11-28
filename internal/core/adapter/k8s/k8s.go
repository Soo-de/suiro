package k8s

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Soo-de/suiro/internal/core"
	"github.com/Soo-de/suiro/internal/model"
)

// New returns the adapter for model.SourceGit snapshots: rendered
// Kubernetes objects and expectation entries.
func New() core.Adapter {
	return adapter{kind: model.SourceGit}
}

// NewCluster returns the adapter for model.SourceCluster snapshots, which
// hold Namespace objects only.
func NewCluster() core.Adapter {
	return adapter{kind: model.SourceCluster}
}

type adapter struct{ kind model.SourceKind }

func (a adapter) Kind() model.SourceKind { return a.kind }

// Translate makes one pass over the objects, so its cost is linear in the
// size of the snapshot, plus the sorting of the result.
func (a adapter) Translate(ctx context.Context, s model.Snapshot) (model.Fragment, error) {
	fail := func(err error) (model.Fragment, error) {
		return model.Fragment{}, fmt.Errorf("k8s: translate %s: %w", s.Key.Source, err)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	// The whole snapshot is checked first: one of another kind yields no
	// partial fragment.
	for _, o := range s.Objects {
		if !a.reads(o.Kind) {
			return fail(fmt.Errorf("object %s does not belong in a %s snapshot", o.Key, a.kind))
		}
	}
	t := translation{fragment: model.Fragment{Source: s.Key.Source}}
	for _, o := range s.Objects {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		t.object(o)
	}
	t.sort()
	return t.fragment, nil
}

// reads reports whether a snapshot of the adapter's source kind can hold an
// object of kind.
func (a adapter) reads(kind model.ObjectKind) bool {
	if a.kind == model.SourceCluster {
		return kind == model.KindNamespace
	}
	return kind == model.KindExpectation || strings.HasPrefix(string(kind), "k8s:")
}

// translation collects the fragment of one snapshot.
type translation struct{ fragment model.Fragment }

var (
	ignoredKinds     = model.IgnoredKinds()
	unsupportedKinds = model.UnsupportedKinds()
)

// object adds what o becomes to the fragment: nothing for an ignored kind,
// else its translation or a problem.
func (t *translation) object(o model.Object) {
	if slices.Contains(ignoredKinds, o.Kind) {
		return
	}
	// Checked before the translated kinds: Ingress keeps fields but is
	// outside the model.
	if reason, ok := unsupportedKinds[o.Kind]; ok {
		t.flag(o, reason, "", "this kind changes what the network allows and is outside the model")
		return
	}
	switch o.Kind {
	case model.KindNamespace:
		t.namespace(o)
	case model.KindPod, model.KindDeployment, model.KindStatefulSet, model.KindDaemonSet,
		model.KindReplicaSet, model.KindJob, model.KindCronJob:
		t.workload(o)
	case model.KindService:
		t.service(o)
	case model.KindNetworkPolicy:
		t.networkPolicy(o)
	default:
		t.unknownKind(o)
	}
}

// unknownKind flags a kind the model does not know, with another reason when
// it creates pods.
func (t *translation) unknownKind(o model.Object) {
	creates, err := hasPodTemplate(o.Body)
	switch {
	case err != nil:
		t.invalid(o, "")
	case creates:
		t.flag(o, model.ReasonWorkloadKindUnsupported, "", "this kind creates pods the model does not see")
	default:
		t.flag(o, model.ReasonKindUnrecognized, "", "unrecognized kind")
	}
}

// flag adds a problem about o; locator names the field, "" the whole object.
func (t *translation) flag(o model.Object, reason model.ReasonCode, locator, message string) {
	origin := o.Origin
	origin.Locator = locator
	t.fragment.Problems = append(t.fragment.Problems,
		model.Problem{Reason: reason, Subject: string(o.Key), Message: message, Origin: &origin})
}

// invalid flags o as one the API server would reject; locator is "" for a
// body that does not parse. The caller adds nothing else for o.
func (t *translation) invalid(o model.Object, locator string) {
	message := "the API server would reject this field"
	if locator == "" {
		message = "the body does not parse as an object of this kind"
	}
	t.flag(o, model.ReasonObjectInvalid, locator, message)
}

// sort puts every list of the fragment in its canonical order.
func (t *translation) sort() {
	f := &t.fragment
	slices.SortStableFunc(f.Workloads, func(a, b model.Workload) int { return cmp.Compare(a.Key, b.Key) })
	slices.SortStableFunc(f.Namespaces, func(a, b model.Namespace) int { return cmp.Compare(a.Name, b.Name) })
	slices.SortStableFunc(f.Gates, func(a, b model.Gate) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortStableFunc(f.Expectations, func(a, b model.Expectation) int { return cmp.Compare(a.ID, b.ID) })
	// Every problem the adapter raises has an Origin.
	slices.SortStableFunc(f.Problems, func(a, b model.Problem) int {
		return cmp.Or(
			cmp.Compare(a.Reason, b.Reason),
			cmp.Compare(a.Subject, b.Subject),
			cmp.Compare(a.Origin.File, b.Origin.File),
			cmp.Compare(a.Origin.Line, b.Origin.Line),
			cmp.Compare(a.Origin.Locator, b.Origin.Locator),
			cmp.Compare(a.Message, b.Message),
		)
	})
}
