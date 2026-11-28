package k8s_test

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Soo-de/suiro/internal/core"
	"github.com/Soo-de/suiro/internal/core/adapter/k8s"
	"github.com/Soo-de/suiro/internal/model"
	"github.com/Soo-de/suiro/internal/testutil"
)

const (
	source = model.SourceID("shop/api")
	file   = "manifests.yaml"
)

// Helpers. Every translation made through translate is also held to the
// invariants all fragments share.

func snapshot(t *testing.T, docs string) model.Snapshot {
	t.Helper()
	return testutil.Snapshot(t, model.ReadKey{Source: source}, file, docs)
}

func git(t *testing.T, docs string) (model.Snapshot, model.Fragment) {
	t.Helper()
	s := snapshot(t, docs)
	return s, translate(t, k8s.New(), s)
}

func cluster(t *testing.T, docs string) (model.Snapshot, model.Fragment) {
	t.Helper()
	s := snapshot(t, docs)
	return s, translate(t, k8s.NewCluster(), s)
}

func translate(t *testing.T, a core.Adapter, s model.Snapshot) model.Fragment {
	t.Helper()
	before := encode(t, s)
	bodies := make([][]byte, len(s.Objects))
	for i, o := range s.Objects {
		bodies[i] = bytes.Clone(o.Body)
	}
	f, err := a.Translate(context.Background(), s)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	again, err := a.Translate(context.Background(), s)
	if err != nil {
		t.Fatalf("Translate, second run: %v", err)
	}
	if !reflect.DeepEqual(f, again) {
		t.Errorf("two runs differ:\n%s\n%s", encode(t, f), encode(t, again))
	}
	for i, o := range s.Objects {
		if !bytes.Equal(o.Body, bodies[i]) {
			t.Errorf("%s: Translate changed the body", o.Key)
		}
	}
	if after := encode(t, s); after != before {
		t.Errorf("Translate changed the snapshot:\n%s\n%s", before, after)
	}
	if err := f.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
	if f.Source != s.Key.Source {
		t.Errorf("Source = %q, want %q", f.Source, s.Key.Source)
	}
	checkSorted(t, f)
	checkProblems(t, s, f)
	checkAccounted(t, s, f)
	return f
}

func encode(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func checkSorted(t *testing.T, f model.Fragment) {
	t.Helper()
	check := func(list string, sorted bool) {
		if !sorted {
			t.Errorf("%s are not sorted: %s", list, encode(t, f))
		}
	}
	check("workloads", slices.IsSortedFunc(f.Workloads, func(a, b model.Workload) int { return cmp.Compare(a.Key, b.Key) }))
	check("namespaces", slices.IsSortedFunc(f.Namespaces, func(a, b model.Namespace) int { return cmp.Compare(a.Name, b.Name) }))
	check("gates", slices.IsSortedFunc(f.Gates, func(a, b model.Gate) int { return cmp.Compare(a.ID, b.ID) }))
	check("expectations", slices.IsSortedFunc(f.Expectations, func(a, b model.Expectation) int { return cmp.Compare(a.ID, b.ID) }))
	check("problems", slices.IsSortedFunc(f.Problems, func(a, b model.Problem) int {
		var x, y model.Origin
		if a.Origin != nil {
			x = *a.Origin
		}
		if b.Origin != nil {
			y = *b.Origin
		}
		return cmp.Or(cmp.Compare(a.Reason, b.Reason), cmp.Compare(a.Subject, b.Subject),
			cmp.Compare(x.File, y.File), cmp.Compare(x.Line, y.Line), cmp.Compare(x.Locator, y.Locator),
			cmp.Compare(a.Message, b.Message))
	}))
}

// checkProblems checks that every problem has an adapter's reason, names an
// object and points at where that object came from.
func checkProblems(t *testing.T, s model.Snapshot, f model.Fragment) {
	t.Helper()
	origins := make(map[string]model.Origin, len(s.Objects))
	for _, o := range s.Objects {
		origins[string(o.Key)] = o.Origin
	}
	for _, p := range f.Problems {
		if !slices.Contains(model.Reasons()[p.Reason].RaisedBy, model.ProducerAdapter) {
			t.Errorf("reason %q is not one the adapter raises", p.Reason)
		}
		want, ok := origins[p.Subject]
		switch {
		case !ok:
			t.Errorf("problem %s: subject %q is not an object of the snapshot", p.Reason, p.Subject)
		case p.Origin == nil:
			t.Errorf("problem %s of %s: no origin", p.Reason, p.Subject)
		default:
			got := *p.Origin
			got.Locator = want.Locator
			if got != want {
				t.Errorf("problem %s of %s: origin %+v, want %+v", p.Reason, p.Subject, *p.Origin, want)
			}
		}
	}
}

func checkAccounted(t *testing.T, s model.Snapshot, f model.Fragment) {
	t.Helper()
	seen := make(map[model.ObjectKey]bool)
	for _, w := range f.Workloads {
		seen[model.ObjectKey(w.Key)] = true
	}
	for _, n := range f.Namespaces {
		seen[model.NewObjectKey(model.KindNamespace, "", n.Name)] = true
	}
	for _, g := range f.Gates {
		seen[g.Object] = true
	}
	for _, e := range f.Expectations {
		seen[model.NewObjectKey(model.KindExpectation, "", e.ID)] = true
	}
	for _, p := range f.Problems {
		seen[model.ObjectKey(p.Subject)] = true
	}
	ignored := model.IgnoredKinds()
	for _, o := range s.Objects {
		if !seen[o.Key] && !slices.Contains(ignored, o.Kind) {
			t.Errorf("%s is in neither the fragment nor a problem", o.Key)
		}
	}
}

// problem is what a test states about a problem; its message is free.
type problem struct {
	reason  model.ReasonCode
	subject model.ObjectKey
	locator string
}

func problems(f model.Fragment) []problem {
	var out []problem
	for _, p := range f.Problems {
		q := problem{reason: p.Reason, subject: model.ObjectKey(p.Subject)}
		if p.Origin != nil {
			q.locator = p.Origin.Locator
		}
		out = append(out, q)
	}
	return out
}

// anyLocator marks a problem whose locator the contract does not fix.
const anyLocator = "*any*"

// wantProblems compares the problems of f with want, in the fragment's order.
func wantProblems(t *testing.T, f model.Fragment, want ...problem) {
	t.Helper()
	got := problems(f)
	if len(got) == len(want) {
		for i := range got {
			if want[i].locator == anyLocator {
				got[i].locator = anyLocator
			}
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("problems\n got %+v\nwant %+v", got, want)
	}
}

// same compares two values by their JSON, in which nil and empty lists and
// maps of omitempty fields are one.
func same(t *testing.T, what string, got, want any) {
	t.Helper()
	if g, w := encode(t, got), encode(t, want); g != w {
		t.Errorf("%s\n got %s\nwant %s", what, g, w)
	}
}

// wantEmptyModel checks that f holds nothing but problems.
func wantEmptyModel(t *testing.T, f model.Fragment) {
	t.Helper()
	f.Problems = nil
	same(t, "fragment without problems", f, model.Fragment{Source: f.Source})
}

// under reports whether locator names path or a field inside it.
func under(locator, path string) bool {
	rest, ok := strings.CutPrefix(locator, path)
	return ok && (rest == "" || rest[0] == '.' || rest[0] == '[')
}

// wantInvalid checks that doc, one object, yields nothing but one
// object-invalid problem whose locator names path or a field inside it.
func wantInvalid(t *testing.T, a core.Adapter, doc, path string) {
	t.Helper()
	s := snapshot(t, doc)
	if len(s.Objects) != 1 {
		t.Fatalf("want one object, got %d", len(s.Objects))
	}
	f := translate(t, a, s)
	wantEmptyModel(t, f)
	wantProblems(t, f, problem{model.ReasonObjectInvalid, s.Objects[0].Key, anyLocator})
	if got := problems(f); len(got) == 1 && !under(got[0].locator, path) {
		t.Errorf("locator %q does not name %q", got[0].locator, path)
	}
}

// at is the origin of o with locator.
func at(o model.Object, locator string) model.Origin {
	origin := o.Origin
	origin.Locator = locator
	return origin
}

// only returns the snapshot's one object.
func only(t *testing.T, s model.Snapshot) model.Object {
	t.Helper()
	if len(s.Objects) != 1 {
		t.Fatalf("want one object, got %d", len(s.Objects))
	}
	return s.Objects[0]
}

func indent(n int, text string) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(strings.Trim(text, "\n"), "\n", "\n"+pad)
}

// Basics.

func TestKind(t *testing.T) {
	if got := k8s.New().Kind(); got != model.SourceGit {
		t.Errorf("New().Kind() = %q, want git", got)
	}
	if got := k8s.NewCluster().Kind(); got != model.SourceCluster {
		t.Errorf("NewCluster().Kind() = %q, want cluster", got)
	}
}

func TestDoneContext(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer stop()

	s := snapshot(t, "apiVersion: v1\nkind: Namespace\nmetadata: {name: shop}")
	empty := snapshot(t, "")
	for _, tt := range []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"canceled", canceled, context.Canceled},
		{"deadline", expired, context.DeadlineExceeded},
	} {
		for _, a := range []core.Adapter{k8s.New(), k8s.NewCluster()} {
			for _, in := range []model.Snapshot{s, empty} {
				t.Run(fmt.Sprintf("%s %s %d objects", tt.name, a.Kind(), len(in.Objects)), func(t *testing.T) {
					if _, err := a.Translate(tt.ctx, in); !errors.Is(err, tt.want) {
						t.Errorf("err = %v, want %v", err, tt.want)
					}
				})
			}
		}
	}
}

func TestSnapshotOfAnotherKind(t *testing.T) {
	object := func(kind model.ObjectKind, namespace, name, body string) model.Object {
		return model.Object{
			Kind:   kind,
			Key:    model.NewObjectKey(kind, namespace, name),
			Origin: model.Origin{Source: source},
			Body:   json.RawMessage(body),
		}
	}
	namespace := object(model.KindNamespace, "", "shop",
		`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"shop"}}`)
	vpc := object(model.KindVPC, "", "vpc-1", `{"VpcId":"vpc-1"}`)
	other := object("suiro:core/Other", "", "x", `{"name":"x"}`)
	deployment := object(model.KindDeployment, "shop", "api",
		`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"shop"},"spec":{"template":{"spec":{"containers":[{"name":"api"}]}}}}`)
	configMap := object("k8s:core/ConfigMap", "shop", "c",
		`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"c","namespace":"shop"}}`)
	expectation := object(model.KindExpectation, "", "shop/api#a", `{"name":"a"}`)

	for _, tt := range []struct {
		name    string
		adapter core.Adapter
		objects []model.Object
	}{
		{"git, aws object", k8s.New(), []model.Object{vpc}},
		{"git, aws object after a namespace", k8s.New(), []model.Object{namespace, vpc}},
		{"git, another suiro kind", k8s.New(), []model.Object{other}},
		{"cluster, deployment", k8s.NewCluster(), []model.Object{deployment}},
		{"cluster, deployment after a namespace", k8s.NewCluster(), []model.Object{namespace, deployment}},
		{"cluster, ignored kind", k8s.NewCluster(), []model.Object{configMap}},
		{"cluster, expectation", k8s.NewCluster(), []model.Object{expectation}},
		{"cluster, aws object", k8s.NewCluster(), []model.Object{vpc}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := model.Snapshot{Key: model.ReadKey{Source: source}, ContractVersion: model.ContractVersion, Objects: tt.objects}
			if _, err := tt.adapter.Translate(context.Background(), s); err == nil {
				t.Error("no error")
			}
		})
	}
}

func TestEmptySnapshot(t *testing.T) {
	for _, a := range []core.Adapter{k8s.New(), k8s.NewCluster()} {
		f := translate(t, a, snapshot(t, ""))
		same(t, string(a.Kind()), f, model.Fragment{Source: source})
	}
}

func TestSnapshotProblemsAreNotCopied(t *testing.T) {
	for _, a := range []core.Adapter{k8s.New(), k8s.NewCluster()} {
		for _, docs := range []string{"", "apiVersion: v1\nkind: Namespace\nmetadata: {name: shop}"} {
			s := snapshot(t, docs)
			s.Problems = []model.Problem{
				{Reason: model.ReasonRenderFailed, Subject: string(source), Message: "render failed"},
				{Reason: model.ReasonKindUnrecognized, Subject: "k8s:x.io/Y/a"},
			}
			f := translate(t, a, s)
			if len(f.Problems) != 0 {
				t.Errorf("%s: problems %+v, want none", a.Kind(), f.Problems)
			}
		}
	}
}

// TestListOrder mixes objects whose key order differs from the order the
// fragment's lists must have; translate checks the order.
func TestListOrder(t *testing.T) {
	_, f := git(t, `
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: b, namespace: shop}
spec:
  podSelector: {}
  policyTypes: [Ingress, Egress]
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: a, namespace: shop}
spec:
  podSelector: {}
  policyTypes: [Ingress, Egress]
---
apiVersion: v1
kind: Service
metadata: {name: z, namespace: shop}
spec: {selector: {app: z}, ports: [{port: 81}]}
---
apiVersion: v1
kind: Service
metadata: {name: external, namespace: shop}
spec: {type: ExternalName}
---
apiVersion: v1
kind: Pod
metadata: {name: z, namespace: shop}
spec: {containers: [{name: c}]}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: a, namespace: shop}
spec: {template: {spec: {hostNetwork: true, containers: [{name: c, ports: [{containerPort: 80}]}]}}}
---
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata: {name: r, namespace: shop}
spec: {template: {spec: {containers: [{name: c}]}}}
---
apiVersion: v1
kind: Namespace
metadata: {name: b}
---
apiVersion: v1
kind: Namespace
metadata: {name: a}
---
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
metadata: {name: c, namespace: shop}
`)
	if len(f.Gates) != 5 || len(f.Workloads) != 2 || len(f.Namespaces) != 2 || len(f.Problems) != 6 {
		t.Errorf("got %d gates, %d workloads, %d namespaces, %d problems; want 5, 2, 2, 6: %s",
			len(f.Gates), len(f.Workloads), len(f.Namespaces), len(f.Problems), encode(t, f))
	}
}

// Kinds.

// identity is a manifest of kind that holds identity only.
func identity(kind model.ObjectKind) string {
	group, name, _ := strings.Cut(strings.TrimPrefix(string(kind), "k8s:"), "/")
	apiVersion := "v1"
	if group != "core" {
		apiVersion = group + "/v1"
	}
	return fmt.Sprintf("apiVersion: %s\nkind: %s\nmetadata: {name: x, namespace: shop}\n", apiVersion, name)
}

func TestIgnoredKinds(t *testing.T) {
	for _, kind := range model.IgnoredKinds() {
		t.Run(string(kind), func(t *testing.T) {
			s, f := git(t, identity(kind))
			if got := only(t, s).Kind; got != kind {
				t.Fatalf("built %s", got)
			}
			same(t, "fragment", f, model.Fragment{Source: source})
		})
	}
}

func TestUnsupportedKinds(t *testing.T) {
	for kind, reason := range model.UnsupportedKinds() {
		t.Run(string(kind), func(t *testing.T) {
			s, f := git(t, identity(kind))
			o := only(t, s)
			if o.Kind != kind {
				t.Fatalf("built %s", o.Kind)
			}
			wantEmptyModel(t, f)
			wantProblems(t, f, problem{reason, o.Key, anyLocator})
		})
	}
}

func TestIngress(t *testing.T) {
	for name, spec := range map[string]string{
		"no spec":    "",
		"no class":   "spec: {rules: [{host: example.com}]}",
		"alb class":  "spec: {ingressClassName: alb}",
		"other kind": "spec: {ingressClassName: nginx}",
	} {
		t.Run(name, func(t *testing.T) {
			s, f := git(t, "apiVersion: networking.k8s.io/v1\nkind: Ingress\nmetadata: {name: web, namespace: shop}\n"+spec)
			wantEmptyModel(t, f)
			wantProblems(t, f, problem{model.ReasonAWSControllerEffect, only(t, s).Key, anyLocator})
		})
	}
}

func TestUnknownKinds(t *testing.T) {
	for _, tt := range []struct {
		name string
		doc  string
		want model.ReasonCode
	}{
		{"rollout with a pod template", `
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata: {name: api, namespace: shop}
spec:
  template:
    spec:
      containers: [{name: api, image: example/api}]
`, model.ReasonWorkloadKindUnsupported},
		{"the containers key alone counts", `
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata: {name: api, namespace: shop}
spec: {template: {spec: {containers: []}}}
`, model.ReasonWorkloadKindUnsupported},
		{"certificate", `
apiVersion: cert-manager.io/v1
kind: Certificate
metadata: {name: tls, namespace: shop}
spec: {secretName: tls, dnsNames: [example.com]}
`, model.ReasonKindUnrecognized},
		{"template without containers", `
apiVersion: example.com/v1
kind: Thing
metadata: {name: t, namespace: shop}
spec: {template: {spec: {volumes: []}}}
`, model.ReasonKindUnrecognized},
		{"cluster scoped", `
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata: {name: issuer}
`, model.ReasonKindUnrecognized},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, f := git(t, tt.doc)
			wantEmptyModel(t, f)
			wantProblems(t, f, problem{tt.want, only(t, s).Key, anyLocator})
		})
	}
}

// Namespaces. The automatic label is the one of the Namespaces page,
// "Automatic labelling".

const nameLabel = "kubernetes.io/metadata.name"

func TestNamespaces(t *testing.T) {
	long := strings.Repeat("a", 63)
	for _, tt := range []struct {
		name     string
		metadata string
		want     map[string]string
	}{
		{"labels", "{name: shop, labels: {team: a}}", map[string]string{"team": "a", nameLabel: "shop"}},
		{"no labels", "{name: shop}", map[string]string{nameLabel: "shop"}},
		{"written name label is replaced", "{name: shop, labels: {kubernetes.io/metadata.name: other}}",
			map[string]string{nameLabel: "shop"}},
		{"prefixed key, empty and longest values", `{name: shop, labels: {example.com/team: "", long: ` + long + `}}`,
			map[string]string{"example.com/team": "", "long": long, nameLabel: "shop"}},
	} {
		for _, a := range []core.Adapter{k8s.New(), k8s.NewCluster()} {
			t.Run(tt.name+" "+string(a.Kind()), func(t *testing.T) {
				s := snapshot(t, "apiVersion: v1\nkind: Namespace\nmetadata: "+tt.metadata)
				f := translate(t, a, s)
				same(t, "fragment", f, model.Fragment{Source: source, Namespaces: []model.Namespace{
					{Name: "shop", Labels: tt.want, Origin: only(t, s).Origin},
				}})
			})
		}
	}
}

func TestNamespaceInvalidLabels(t *testing.T) {
	for name, labels := range map[string]string{
		"key starts with a dash": `{"-x": a}`,
		"key with two slashes":   `{"a/b/c": a}`,
		"value of 64 characters": `{team: ` + strings.Repeat("a", 64) + `}`,
		"value with a space":     `{team: "a b"}`,
	} {
		for _, a := range []core.Adapter{k8s.New(), k8s.NewCluster()} {
			t.Run(name+" "+string(a.Kind()), func(t *testing.T) {
				s := snapshot(t, "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: shop\n  labels: "+labels)
				f := translate(t, a, s)
				wantEmptyModel(t, f)
				wantProblems(t, f, problem{model.ReasonObjectInvalid, only(t, s).Key, "metadata.labels"})
			})
		}
	}
}

func TestInvalidNamespaceKeepsTheOthers(t *testing.T) {
	s, f := cluster(t, `
apiVersion: v1
kind: Namespace
metadata: {name: bad, labels: {"-x": a}}
---
apiVersion: v1
kind: Namespace
metadata: {name: good}
`)
	if len(f.Namespaces) != 1 || f.Namespaces[0].Name != "good" {
		t.Errorf("namespaces %+v, want good alone", f.Namespaces)
	}
	wantProblems(t, f, problem{model.ReasonObjectInvalid, s.Objects[0].Key, "metadata.labels"})
}

// Workloads.

// workloadKind says where a kind keeps its pod template.
type workloadKind struct {
	apiVersion, kind string
	// metadata and spec are the paths of the template's metadata and spec.
	metadata, spec string
}

var workloadKinds = []workloadKind{
	{"apps/v1", "Deployment", "spec.template.metadata", "spec.template.spec"},
	{"apps/v1", "StatefulSet", "spec.template.metadata", "spec.template.spec"},
	{"apps/v1", "DaemonSet", "spec.template.metadata", "spec.template.spec"},
	{"apps/v1", "ReplicaSet", "spec.template.metadata", "spec.template.spec"},
	{"batch/v1", "Job", "spec.template.metadata", "spec.template.spec"},
	{"batch/v1", "CronJob", "spec.jobTemplate.spec.template.metadata", "spec.jobTemplate.spec.template.spec"},
	{"v1", "Pod", "metadata", "spec"},
}

// manifest writes an object of k named web in shop, whose pod template has
// the given labels (a YAML flow mapping) and spec (block YAML).
func (k workloadKind) manifest(labels, spec string) string {
	head := fmt.Sprintf("apiVersion: %s\nkind: %s\n", k.apiVersion, k.kind)
	if k.kind == "Pod" {
		return head + "metadata:\n  name: web\n  namespace: shop\n  labels: " + labels + "\nspec:\n" + indent(2, spec) + "\n"
	}
	body := "template:\n  metadata:\n    labels: " + labels + "\n  spec:\n" + indent(4, spec)
	if k.kind == "CronJob" {
		body = "jobTemplate:\n  spec:\n" + indent(4, body)
	}
	return head + "metadata: {name: web, namespace: shop}\nspec:\n" + indent(2, body) + "\n"
}

func (k workloadKind) objectKind() model.ObjectKind {
	group := "core"
	if g, _, ok := strings.Cut(k.apiVersion, "/"); ok {
		group = g
	}
	return model.ObjectKind("k8s:" + group + "/" + k.kind)
}

// workload is what an object of k named web in shop becomes, before its
// labels, ports and placement.
func (k workloadKind) workload(o model.Object) model.Workload {
	return model.Workload{
		Key:  model.EndpointKey(model.NewObjectKey(k.objectKind(), "shop", "web")),
		Kind: k.objectKind(), Namespace: "shop", Name: "web", Origin: o.Origin,
	}
}

const fullPodSpec = `
nodeSelector: {disktype: ssd}
nodeName: node-1
tolerations:
- {key: dedicated, operator: Equal, value: web, effect: NoSchedule}
- {operator: Exists}
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
      - matchExpressions:
        - {key: zone, operator: In, values: [b, a]}
      - matchExpressions:
        - {key: gpu, operator: Exists}
        - {key: cores, operator: Gt, values: ["4"]}
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 1
      preference:
        matchExpressions:
        - {key: preferred, operator: Exists}
  podAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
    - labelSelector: {matchLabels: {app: cache}}
      namespaces: [data, shop]
      topologyKey: kubernetes.io/hostname
    - namespaceSelector: {}
      labelSelector:
        matchExpressions:
        - {key: app, operator: In, values: [queue]}
      topologyKey: topology.kubernetes.io/zone
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 1
      podAffinityTerm: {topologyKey: preferred}
  podAntiAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
    - labelSelector: {matchLabels: {app: web}}
      topologyKey: kubernetes.io/hostname
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 1
      podAffinityTerm: {topologyKey: preferred}
containers:
- name: web
  image: example/web
  ports:
  - {name: http, containerPort: 8080}
  - {containerPort: 9000, protocol: SCTP}
  - {containerPort: 5353, protocol: UDP}
- name: sidecar
- name: metrics
  ports:
  - {name: metrics, containerPort: 65535, protocol: TCP}
  - {name: abcdefghijklmno, containerPort: 1}
`

func fullPlacement() model.Placement {
	return model.Placement{
		NodeSelector: map[string]string{"disktype": "ssd"},
		NodeName:     "node-1",
		Tolerations: []model.Toleration{
			{Key: "dedicated", Operator: "Equal", Value: "web", Effect: "NoSchedule"},
			{Operator: "Exists"},
		},
		NodeTerms: []model.NodeSelectorTerm{
			{MatchExpressions: []model.LabelRequirement{{Key: "zone", Operator: "In", Values: []string{"b", "a"}}}},
			{MatchExpressions: []model.LabelRequirement{
				{Key: "gpu", Operator: "Exists"},
				{Key: "cores", Operator: "Gt", Values: []string{"4"}},
			}},
		},
		PodAffinity: []model.PodAffinityTerm{
			{
				LabelSelector: &model.LabelSelector{MatchLabels: map[string]string{"app": "cache"}},
				Namespaces:    []string{"data", "shop"},
				TopologyKey:   "kubernetes.io/hostname",
			},
			{
				NamespaceSelector: &model.LabelSelector{},
				LabelSelector: &model.LabelSelector{MatchExpressions: []model.LabelRequirement{
					{Key: "app", Operator: "In", Values: []string{"queue"}},
				}},
				TopologyKey: "topology.kubernetes.io/zone",
			},
		},
		PodAntiAffinity: []model.PodAffinityTerm{{
			LabelSelector: &model.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			TopologyKey:   "kubernetes.io/hostname",
		}},
	}
}

func TestWorkloadKinds(t *testing.T) {
	for _, k := range workloadKinds {
		t.Run(k.kind, func(t *testing.T) {
			s, f := git(t, k.manifest("{app: web, tier: front}", fullPodSpec))
			o := only(t, s)
			want := k.workload(o)
			want.PodLabels = map[string]string{"app": "web", "tier": "front"}
			want.Placement = fullPlacement()
			want.Ports = []model.ContainerPort{
				{Container: "web", Name: "http", Port: 8080, Protocol: model.ProtocolTCP},
				{Container: "web", Port: 5353, Protocol: model.ProtocolUDP},
				{Container: "metrics", Name: "metrics", Port: 65535, Protocol: model.ProtocolTCP},
				{Container: "metrics", Name: "abcdefghijklmno", Port: 1, Protocol: model.ProtocolTCP},
			}
			same(t, "workloads", f.Workloads, []model.Workload{want})
			if len(f.Gates)+len(f.Namespaces)+len(f.Expectations) != 0 {
				t.Errorf("more than a workload: %s", encode(t, f))
			}
			if k.kind == "Pod" {
				wantProblems(t, f, problem{model.ReasonStandalonePod, o.Key, anyLocator})
			} else {
				wantProblems(t, f)
			}
		})
	}
}

func TestMinimalWorkload(t *testing.T) {
	for _, k := range workloadKinds {
		t.Run(k.kind, func(t *testing.T) {
			s, f := git(t, k.manifest("{}", "hostNetwork: false\ncontainers: [{name: c}]"))
			same(t, "workloads", f.Workloads, []model.Workload{k.workload(only(t, s))})
		})
	}
}

func TestHostPort(t *testing.T) {
	for _, k := range workloadKinds {
		t.Run(k.kind, func(t *testing.T) {
			s, f := git(t, k.manifest("{}", `
containers:
- name: a
  ports:
  - {containerPort: 80}
- name: b
  ports:
  - {containerPort: 9000, protocol: SCTP}
  - {containerPort: 9100, hostPort: 9100}
  - {containerPort: 8080, hostPort: 80, protocol: UDP}
`))
			o := only(t, s)
			want := k.workload(o)
			want.Ports = []model.ContainerPort{
				{Container: "a", Port: 80, Protocol: model.ProtocolTCP},
				{Container: "b", Port: 9100, HostPort: 9100, Protocol: model.ProtocolTCP},
				{Container: "b", Port: 8080, HostPort: 80, Protocol: model.ProtocolUDP},
			}
			same(t, "workloads", f.Workloads, []model.Workload{want})
			expected := []problem{
				{model.ReasonHostPort, o.Key, k.spec + ".containers[1].ports[1]"},
				{model.ReasonHostPort, o.Key, k.spec + ".containers[1].ports[2]"},
			}
			if k.kind == "Pod" {
				expected = append(expected, problem{model.ReasonStandalonePod, o.Key, anyLocator})
			}
			wantProblems(t, f, expected...)
		})
	}
}

func TestHostNetwork(t *testing.T) {
	for _, k := range workloadKinds {
		t.Run(k.kind, func(t *testing.T) {
			s, f := git(t, k.manifest("{}", `
hostNetwork: true
containers:
- name: a
  ports:
  - {containerPort: 80}
  - {containerPort: 81, hostPort: 81}
`))
			o := only(t, s)
			want := k.workload(o)
			want.HostNetwork = true
			want.Ports = []model.ContainerPort{
				{Container: "a", Port: 80, HostPort: 80, Protocol: model.ProtocolTCP},
				{Container: "a", Port: 81, HostPort: 81, Protocol: model.ProtocolTCP},
			}
			same(t, "workloads", f.Workloads, []model.Workload{want})
			expected := []problem{
				{model.ReasonHostNetwork, o.Key, k.spec + ".hostNetwork"},
				{model.ReasonHostPort, o.Key, k.spec + ".containers[0].ports[0]"},
				{model.ReasonHostPort, o.Key, k.spec + ".containers[0].ports[1]"},
			}
			if k.kind == "Pod" {
				expected = append(expected, problem{model.ReasonStandalonePod, o.Key, anyLocator})
			}
			wantProblems(t, f, expected...)
		})
	}
}

func TestInvalidWorkloads(t *testing.T) {
	port := func(p string) string { return "containers: [{name: c, ports: [" + p + "]}]" }
	for _, tt := range []struct {
		name   string
		labels string
		spec   string
		// path is below the pod template's spec, or its metadata for labels.
		path string
	}{
		{"containerPort 0", "{}", port("{containerPort: 0}"), "containers[0].ports[0]"},
		{"no containerPort", "{}", port("{name: http}"), "containers[0].ports[0]"},
		{"containerPort 70000", "{}", port("{containerPort: 70000}"), "containers[0].ports[0]"},
		{"protocol in lower case", "{}", port("{containerPort: 80, protocol: tcp}"), "containers[0].ports[0]"},
		{"port name in upper case", "{}", port("{containerPort: 80, name: HTTP}"), "containers[0].ports[0]"},
		{"port name without a letter", "{}", port(`{containerPort: 80, name: "8080"}`), "containers[0].ports[0]"},
		{"port name of 16 characters", "{}", port("{containerPort: 80, name: abcdefghijklmnop}"), "containers[0].ports[0]"},
		{"port name twice in a container", "{}",
			port("{containerPort: 80, name: http}, {containerPort: 81, name: http}"), "containers[0].ports[1]"},
		{"no containers", "{}", "nodeName: node-1", "containers"},
		{"empty containers", "{}", "containers: []", "containers"},
		{"container without a name", "{}", "containers: [{name: a}, {ports: []}]", "containers[1]"},
		{"container name twice", "{}", "containers: [{name: a}, {name: a}]", "containers[1]"},
		{"hostNetwork with another hostPort", "{}",
			"hostNetwork: true\n" + port("{containerPort: 80, hostPort: 81}"), "containers[0].ports[0]"},
		{"first failing container", "{}",
			"containers: [{name: a, ports: [{containerPort: 0}]}, {name: a}]", "containers[0].ports[0]"},
		{"label key", `{"-x": a}`, "containers: [{name: c}]", "labels"},
		{"label value", `{team: ` + strings.Repeat("a", 64) + `}`, "containers: [{name: c}]", "labels"},
	} {
		for _, k := range []workloadKind{workloadKinds[0], workloadKinds[5], workloadKinds[6]} {
			t.Run(tt.name+" "+k.kind, func(t *testing.T) {
				path := k.spec + "." + tt.path
				if tt.path == "labels" {
					path = k.metadata + ".labels"
				}
				wantInvalid(t, k8s.New(), k.manifest(tt.labels, tt.spec), path)
			})
		}
	}
}

// Every workload kind validates its own template.
func TestInvalidWorkloadOfEachKind(t *testing.T) {
	for _, k := range workloadKinds {
		t.Run(k.kind, func(t *testing.T) {
			// hostNetwork, a host port and a bare Pod would each raise a
			// problem of their own on a valid object.
			doc := k.manifest("{}", "hostNetwork: true\ncontainers: [{name: c, ports: [{containerPort: 70000}]}]")
			wantInvalid(t, k8s.New(), doc, k.spec+".containers[0].ports[0]")
		})
	}
}

func TestWorkloadWithoutNamespace(t *testing.T) {
	for _, k := range workloadKinds {
		t.Run(k.kind, func(t *testing.T) {
			doc := k.manifest("{}", "containers: [{name: c}]")
			doc = strings.Replace(doc, "  namespace: shop\n", "", 1)
			doc = strings.Replace(doc, ", namespace: shop", "", 1)
			wantInvalid(t, k8s.New(), doc, "metadata.namespace")
		})
	}
}
