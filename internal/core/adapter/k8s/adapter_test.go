package k8s_test

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
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
		for i, w := range want {
			if w.locator == anyLocator && i < len(got) {
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

// Services. The cases follow the Service concept page, "Defining a Service",
// "Port definitions", "Multi-port Services" and "Service type".

// service is a manifest of the Service my-service with the given spec.
func service(spec string) string {
	return "apiVersion: v1\nkind: Service\nmetadata: {name: my-service, namespace: default}\nspec:\n" + indent(2, spec) + "\n"
}

// transform is the gate a Service my-service becomes.
func transform(o model.Object, selector map[string]string, mappings ...model.PortMapping) model.Gate {
	return model.Gate{
		ID: model.GateID(o.Key), Layer: model.LayerKubernetes, Type: model.GateTransform,
		Target:   model.GateTarget{Pods: &model.PodTarget{Namespace: "default", Selector: model.LabelSelector{MatchLabels: selector}}},
		Mappings: mappings, Object: o.Key, Origin: o.Origin,
	}
}

func TestServices(t *testing.T) {
	tcp, udp := model.ProtocolTCP, model.ProtocolUDP
	for _, tt := range []struct {
		name  string
		ports string
		want  []model.PortMapping
	}{
		{"the documented my-service", "- {protocol: TCP, port: 80, targetPort: 9376}",
			[]model.PortMapping{{Protocol: tcp, Port: 80, TargetPort: 9376}}},
		{"omitted protocol and targetPort", "- {port: 80}",
			[]model.PortMapping{{Protocol: tcp, Port: 80, TargetPort: 80}}},
		{"named target port", "- {name: name-of-service-port, protocol: TCP, port: 80, targetPort: http-web-svc}",
			[]model.PortMapping{{Name: "name-of-service-port", Protocol: tcp, Port: 80, TargetPortName: "http-web-svc"}}},
		{"udp", "- {protocol: UDP, port: 53, targetPort: 5353}",
			[]model.PortMapping{{Protocol: udp, Port: 53, TargetPort: 5353}}},
		{"multi-port, in written order",
			"- {name: https, protocol: TCP, port: 443, targetPort: 9377}\n- {name: http, protocol: TCP, port: 80, targetPort: 9376}",
			[]model.PortMapping{{Name: "https", Protocol: tcp, Port: 443, TargetPort: 9377}, {Name: "http", Protocol: tcp, Port: 80, TargetPort: 9376}}},
		{"one port number on two protocols",
			"- {name: dns, port: 53}\n- {name: dns-udp, port: 53, protocol: UDP}",
			[]model.PortMapping{{Name: "dns", Protocol: tcp, Port: 53, TargetPort: 53}, {Name: "dns-udp", Protocol: udp, Port: 53, TargetPort: 53}}},
		{"an SCTP port is dropped",
			"- {name: sctp, port: 7, protocol: SCTP}\n- {name: http, port: 80}",
			[]model.PortMapping{{Name: "http", Protocol: tcp, Port: 80, TargetPort: 80}}},
		{"highest ports", "- {port: 65535, targetPort: 65535}",
			[]model.PortMapping{{Protocol: tcp, Port: 65535, TargetPort: 65535}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, f := git(t, service("selector: {app.kubernetes.io/name: MyApp}\nports:\n"+tt.ports))
			want := transform(only(t, s), map[string]string{"app.kubernetes.io/name": "MyApp"}, tt.want...)
			same(t, "fragment", f, model.Fragment{Source: source, Gates: []model.Gate{want}})
		})
	}
}

// A Service whose every port is SCTP keeps its gate.
func TestServiceOnlySCTP(t *testing.T) {
	s, f := git(t, service("selector: {app: a}\nports: [{port: 7, protocol: SCTP}]"))
	want := transform(only(t, s), map[string]string{"app": "a"})
	same(t, "fragment", f, model.Fragment{Source: source, Gates: []model.Gate{want}})
}

func TestServiceTypes(t *testing.T) {
	for _, tt := range []struct {
		kind         string
		loadBalancer bool
	}{
		{"ClusterIP", false},
		{"NodePort", false},
		{"LoadBalancer", true},
	} {
		t.Run(tt.kind, func(t *testing.T) {
			s, f := git(t, service("type: "+tt.kind+"\nselector: {app: a}\nports: [{port: 80}]"))
			o := only(t, s)
			want := transform(o, map[string]string{"app": "a"},
				model.PortMapping{Protocol: model.ProtocolTCP, Port: 80, TargetPort: 80})
			same(t, "gates", f.Gates, []model.Gate{want})
			if tt.loadBalancer {
				wantProblems(t, f, problem{model.ReasonAWSControllerEffect, o.Key, "spec.type"})
			} else {
				wantProblems(t, f)
			}
		})
	}
}

const reasonSelectsNoPods = model.ReasonCode("service-selects-no-pods")

func TestServiceSelectsNoPods(t *testing.T) {
	for name, spec := range map[string]string{
		"ExternalName":                 "type: ExternalName\nexternalName: my.database.example.com",
		"ExternalName with a selector": "type: ExternalName\nexternalName: my.database.example.com\nselector: {app: a}\nports: [{port: 80}]",
		"without selector":             "ports: [{protocol: TCP, port: 80, targetPort: 9376}]",
		"empty selector":               "selector: {}\nports: [{port: 80}]",
	} {
		t.Run(name, func(t *testing.T) {
			s, f := git(t, service(spec))
			wantEmptyModel(t, f)
			wantProblems(t, f, problem{reasonSelectsNoPods, only(t, s).Key, anyLocator})
		})
	}
}

func TestServiceSelectsNoPodsReason(t *testing.T) {
	spec, ok := model.Reasons()[reasonSelectsNoPods]
	if !ok {
		t.Fatalf("no row for %q", reasonSelectsNoPods)
	}
	want := model.ReasonSpec{Level: model.LevelWarning, Scope: model.ScopeNone, RaisedBy: []model.Producer{model.ProducerAdapter}}
	if !reflect.DeepEqual(spec, want) {
		t.Errorf("row %+v, want %+v", spec, want)
	}
}

func TestInvalidServices(t *testing.T) {
	ports := func(p string) string { return "selector: {app: a}\nports: [" + p + "]" }
	for _, tt := range []struct {
		name, spec, path string
	}{
		{"unknown type", "type: Foo\nselector: {app: a}\nports: [{port: 80}]", "spec.type"},
		{"port 0", ports("{port: 0}"), "spec.ports[0]"},
		{"no port", ports("{targetPort: 80}"), "spec.ports[0]"},
		{"port 65536", ports("{port: 65536}"), "spec.ports[0]"},
		{"first of two ports unnamed", ports("{port: 80}, {name: b, port: 81}"), "spec.ports[0]"},
		{"second of two ports unnamed", ports("{name: a, port: 80}, {port: 81}"), "spec.ports[1]"},
		{"duplicate names", ports("{name: a, port: 80}, {name: a, port: 81}"), "spec.ports[1]"},
		{"duplicate port and protocol", ports("{name: a, port: 80}, {name: b, port: 80, protocol: TCP}"), "spec.ports[1]"},
		{"name Web_1", ports("{name: Web_1, port: 80}"), "spec.ports[0]"},
		{"targetPort 70000", ports("{port: 80, targetPort: 70000}"), "spec.ports[0]"},
		{"targetPort as a quoted number", ports(`{port: 80, targetPort: "8080"}`), "spec.ports[0]"},
		{"protocol in lower case", ports("{port: 80, protocol: tcp}"), "spec.ports[0]"},
		{"selector label key", "selector: {\"-x\": a}\nports: [{port: 80}]", "spec.selector"},
		{"selector label value", "selector: {app: \"a b\"}\nports: [{port: 80}]", "spec.selector"},
		// A LoadBalancer would raise a problem of its own on a valid object.
		{"invalid LoadBalancer", "type: LoadBalancer\nselector: {app: a}\nports: [{port: 0}]", "spec.ports[0]"},
		{"invalid without selector", "ports: [{port: 0}]", "spec.ports[0]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantInvalid(t, k8s.New(), service(tt.spec), tt.path)
		})
	}
}

// Network policies. The cases follow the Network Policies concept page and
// the NetworkPolicy API reference.

// netpol is a manifest of the NetworkPolicy p in default with the given spec.
func netpol(spec string) string {
	return "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata: {name: p, namespace: default}\nspec:\n" + indent(2, spec) + "\n"
}

// filter is the gate of one direction of the policy o, in default.
func filter(o model.Object, d model.Direction, selector model.LabelSelector, rules ...model.Rule) model.Gate {
	return model.Gate{
		ID: model.NewGateID(o.Key, d), Layer: model.LayerKubernetes, Type: model.GateFilter, Direction: d,
		Semantics: model.FilterSemantics{Stateful: true, Isolating: true},
		Target:    model.GateTarget{Pods: &model.PodTarget{Namespace: "default", Selector: selector}},
		Rules:     rules, Object: o.Key, Origin: o.Origin,
	}
}

// rule is the allow rule o holds at locator.
func rule(o model.Object, locator string, peers []model.Peer, ports []model.PortRange, named ...model.NamedPort) model.Rule {
	return model.Rule{Action: model.ActionAllow, Peers: peers, Ports: ports, NamedPorts: named, Origin: at(o, locator)}
}

var (
	allPeers = []model.Peer{{All: true}}
	allPorts = []model.PortRange{model.AllPorts()}
)

func tcpPorts(from, to uint16) model.PortRange {
	return model.PortRange{Protocol: model.ProtocolTCP, From: from, To: to}
}

func udpPorts(from, to uint16) model.PortRange {
	return model.PortRange{Protocol: model.ProtocolUDP, From: from, To: to}
}

func labels(key, value string) *model.LabelSelector {
	return &model.LabelSelector{MatchLabels: map[string]string{key: value}}
}

func block(cidr string, except ...string) model.Peer {
	p := model.Peer{Prefix: netip.MustParsePrefix(cidr)}
	for _, e := range except {
		p.Except = append(p.Except, netip.MustParsePrefix(e))
	}
	return p
}

func podPeer(namespaceSelector, podSelector *model.LabelSelector) model.Peer {
	return model.Peer{Pods: &model.PodPeer{Namespace: "default", NamespaceSelector: namespaceSelector, PodSelector: podSelector}}
}

// The example policy of the concept page, "The NetworkPolicy resource".
func TestNetworkPolicyDocumentationExample(t *testing.T) {
	s, f := git(t, netpol(`
podSelector:
  matchLabels:
    role: db
policyTypes:
- Ingress
- Egress
ingress:
- from:
  - ipBlock:
      cidr: 172.17.0.0/16
      except:
      - 172.17.1.0/24
  - namespaceSelector:
      matchLabels:
        project: myproject
  - podSelector:
      matchLabels:
        role: frontend
  ports:
  - protocol: TCP
    port: 6379
egress:
- to:
  - ipBlock:
      cidr: 10.0.0.0/24
  ports:
  - protocol: TCP
    port: 5978
`))
	o := only(t, s)
	selector := *labels("role", "db")
	egress := filter(o, model.Egress, selector,
		rule(o, "spec.egress[0]", []model.Peer{block("10.0.0.0/24")}, []model.PortRange{tcpPorts(5978, 5978)}))
	ingress := filter(o, model.Ingress, selector,
		rule(o, "spec.ingress[0]", []model.Peer{
			block("172.17.0.0/16", "172.17.1.0/24"),
			podPeer(labels("project", "myproject"), nil),
			podPeer(nil, labels("role", "frontend")),
		}, []model.PortRange{tcpPorts(6379, 6379)}))
	same(t, "fragment", f, model.Fragment{Source: source, Gates: []model.Gate{egress, ingress}})
}

func TestNetworkPolicyDirections(t *testing.T) {
	in, out := model.Ingress, model.Egress
	// gates maps each direction wanted to its number of rules.
	for _, tt := range []struct {
		name  string
		spec  string
		gates map[model.Direction]int
	}{
		{"nothing written", "podSelector: {}", map[model.Direction]int{in: 0}},
		{"omitted, egress rule", "podSelector: {}\negress: [{}]", map[model.Direction]int{in: 0, out: 1}},
		{"omitted, ingress and egress rules", "podSelector: {}\ningress: [{}]\negress: [{}]", map[model.Direction]int{in: 1, out: 1}},
		{"omitted, ingress rule", "podSelector: {}\ningress: [{}]", map[model.Direction]int{in: 1}},
		{"omitted, empty egress", "podSelector: {}\negress: []", map[model.Direction]int{in: 0}},
		{"Ingress with egress rules", "podSelector: {}\npolicyTypes: [Ingress]\negress: [{}]", map[model.Direction]int{in: 0}},
		{"Ingress with no ingress", "podSelector: {}\npolicyTypes: [Ingress]", map[model.Direction]int{in: 0}},
		{"Egress alone", "podSelector: {}\npolicyTypes: [Egress]", map[model.Direction]int{out: 0}},
		{"Egress with ingress rules", "podSelector: {}\npolicyTypes: [Egress]\ningress: [{}]", map[model.Direction]int{out: 0}},
		{"Ingress twice", "podSelector: {}\npolicyTypes: [Ingress, Ingress]", map[model.Direction]int{in: 0}},
		{"both", "podSelector: {}\npolicyTypes: [Egress, Ingress]", map[model.Direction]int{in: 0, out: 0}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, f := git(t, netpol(tt.spec))
			o := only(t, s)
			var want []model.Gate
			for _, d := range []model.Direction{out, in} {
				n, ok := tt.gates[d]
				if !ok {
					continue
				}
				g := filter(o, d, model.LabelSelector{})
				for i := range n {
					g.Rules = append(g.Rules, rule(o, fmt.Sprintf("spec.%s[%d]", d, i), allPeers, allPorts))
				}
				want = append(want, g)
			}
			same(t, "fragment", f, model.Fragment{Source: source, Gates: want})
		})
	}
}

func TestNetworkPolicyPodSelector(t *testing.T) {
	s, f := git(t, netpol(`
podSelector:
  matchLabels: {app: web}
  matchExpressions:
  - {key: tier, operator: In, values: [b, a]}
  - {key: env, operator: NotIn, values: [dev]}
  - {key: team, operator: Exists}
  - {key: legacy, operator: DoesNotExist}
`))
	want := filter(only(t, s), model.Ingress, model.LabelSelector{
		MatchLabels: map[string]string{"app": "web"},
		MatchExpressions: []model.LabelRequirement{
			{Key: "tier", Operator: "In", Values: []string{"b", "a"}},
			{Key: "env", Operator: "NotIn", Values: []string{"dev"}},
			{Key: "team", Operator: "Exists"},
			{Key: "legacy", Operator: "DoesNotExist"},
		},
	})
	same(t, "gates", f.Gates, []model.Gate{want})
}

// directions runs check on one rule written as an ingress and as an egress
// rule; far is the name of the rule's peer list.
func directions(t *testing.T, check func(t *testing.T, d model.Direction, far string)) {
	t.Helper()
	t.Run("ingress", func(t *testing.T) { check(t, model.Ingress, "from") })
	t.Run("egress", func(t *testing.T) { check(t, model.Egress, "to") })
}

// ruleSpec writes a policy of direction d alone with the given rules, each a
// flow mapping.
func ruleSpec(d model.Direction, rules ...string) string {
	kind := map[model.Direction]string{model.Ingress: "Ingress", model.Egress: "Egress"}[d]
	spec := "podSelector: {}\npolicyTypes: [" + kind + "]\n" + string(d) + ":"
	for _, r := range rules {
		spec += "\n- " + r
	}
	return netpol(spec)
}

// oneRule checks that a policy with one rule yields one gate holding exactly
// that rule with the given peers and ports, and no problem.
func oneRule(t *testing.T, d model.Direction, written string, peers []model.Peer, ports []model.PortRange, named ...model.NamedPort) {
	t.Helper()
	s, f := git(t, ruleSpec(d, written))
	o := only(t, s)
	want := filter(o, d, model.LabelSelector{}, rule(o, fmt.Sprintf("spec.%s[0]", d), peers, ports, named...))
	same(t, "fragment", f, model.Fragment{Source: source, Gates: []model.Gate{want}})
}

func TestNetworkPolicyPeers(t *testing.T) {
	for _, tt := range []struct {
		name string
		// peers is the rule's from or to list; "" leaves it unwritten.
		peers string
		want  []model.Peer
	}{
		{"unwritten", "", allPeers},
		{"empty", "[]", allPeers},
		{"pod selector", "[{podSelector: {matchLabels: {role: client}}}]",
			[]model.Peer{podPeer(nil, labels("role", "client"))}},
		{"namespace selector", "[{namespaceSelector: {matchLabels: {user: alice}}}]",
			[]model.Peer{podPeer(labels("user", "alice"), nil)}},
		{"both in one element", "[{namespaceSelector: {matchLabels: {user: alice}}, podSelector: {matchLabels: {role: client}}}]",
			[]model.Peer{podPeer(labels("user", "alice"), labels("role", "client"))}},
		{"two elements", "[{namespaceSelector: {matchLabels: {user: alice}}}, {podSelector: {matchLabels: {role: client}}}]",
			[]model.Peer{podPeer(labels("user", "alice"), nil), podPeer(nil, labels("role", "client"))}},
		{"every namespace", "[{namespaceSelector: {}}]", []model.Peer{podPeer(&model.LabelSelector{}, nil)}},
		{"every pod of the namespace", "[{podSelector: {}}]", []model.Peer{podPeer(nil, &model.LabelSelector{})}},
		{"ipBlock with except", "[{ipBlock: {cidr: 172.17.0.0/16, except: [172.17.1.0/24]}}]",
			[]model.Peer{block("172.17.0.0/16", "172.17.1.0/24")}},
		{"ipBlock is masked", "[{ipBlock: {cidr: 10.0.0.1/24, except: [10.0.0.65/26, 10.0.0.200/32]}}]",
			[]model.Peer{block("10.0.0.0/24", "10.0.0.64/26", "10.0.0.200/32")}},
		{"every address", "[{ipBlock: {cidr: 0.0.0.0/0}}]", []model.Peer{block("0.0.0.0/0")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			directions(t, func(t *testing.T, d model.Direction, far string) {
				written := "{}"
				if tt.peers != "" {
					written = "{" + far + ": " + tt.peers + "}"
				}
				oneRule(t, d, written, tt.want, allPorts)
			})
		})
	}
}

func TestNetworkPolicyPorts(t *testing.T) {
	for _, tt := range []struct {
		name string
		// ports is the rule's ports list; "" leaves it unwritten.
		ports string
		want  []model.PortRange
		named []model.NamedPort
	}{
		{"unwritten", "", allPorts, nil},
		{"empty", "[]", allPorts, nil},
		{"a number", "[{port: 6379}]", []model.PortRange{tcpPorts(6379, 6379)}, nil},
		{"udp", "[{protocol: UDP, port: 53}]", []model.PortRange{udpPorts(53, 53)}, nil},
		{"range", "[{port: 32000, endPort: 32768}]", []model.PortRange{tcpPorts(32000, 32768)}, nil},
		{"udp range to the last port", "[{protocol: UDP, port: 1, endPort: 65535}]", []model.PortRange{udpPorts(1, 65535)}, nil},
		{"endPort equal to port", "[{port: 80, endPort: 80}]", []model.PortRange{tcpPorts(80, 80)}, nil},
		{"protocol alone", "[{protocol: UDP}]", []model.PortRange{udpPorts(0, 65535)}, nil},
		{"empty element", "[{}]", []model.PortRange{tcpPorts(0, 65535)}, nil},
		{"written order", "[{port: 443}, {protocol: UDP, port: 53}, {port: 80}]",
			[]model.PortRange{tcpPorts(443, 443), udpPorts(53, 53), tcpPorts(80, 80)}, nil},
		{"named", "[{port: http}]", nil, []model.NamedPort{{Protocol: model.ProtocolTCP, Name: "http"}}},
		{"named and numbered", "[{protocol: UDP, port: dns}, {port: 80}, {port: web}]",
			[]model.PortRange{tcpPorts(80, 80)},
			[]model.NamedPort{{Protocol: model.ProtocolUDP, Name: "dns"}, {Protocol: model.ProtocolTCP, Name: "web"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			directions(t, func(t *testing.T, d model.Direction, _ string) {
				written := "{}"
				if tt.ports != "" {
					written = "{ports: " + tt.ports + "}"
				}
				oneRule(t, d, written, allPeers, tt.want, tt.named...)
			})
		})
	}
}

func TestNetworkPolicyRuleOrder(t *testing.T) {
	directions(t, func(t *testing.T, d model.Direction, _ string) {
		s, f := git(t, ruleSpec(d, "{ports: [{port: 443}]}", "{ports: [{port: 22}]}", "{}"))
		o := only(t, s)
		locator := func(i int) string { return fmt.Sprintf("spec.%s[%d]", d, i) }
		want := filter(o, d, model.LabelSelector{},
			rule(o, locator(0), allPeers, []model.PortRange{tcpPorts(443, 443)}),
			rule(o, locator(1), allPeers, []model.PortRange{tcpPorts(22, 22)}),
			rule(o, locator(2), allPeers, allPorts))
		same(t, "gates", f.Gates, []model.Gate{want})
	})
}

// A rule whose peer or port list dropping empties is dropped whole; it never
// becomes a rule for every peer or port.
func TestNetworkPolicyDroppedRules(t *testing.T) {
	for _, tt := range []struct {
		name string
		// rule holds FAR where "from" or "to" belongs.
		rule string
	}{
		{"only an SCTP port", "{ports: [{protocol: SCTP, port: 7}]}"},
		{"only SCTP with no port", "{ports: [{protocol: SCTP}]}"},
		{"only SCTP ports, one named", "{ports: [{protocol: SCTP, port: diameter}, {protocol: SCTP, port: 7, endPort: 9}]}"},
		{"only SCTP, with a peer", "{FAR: [{podSelector: {}}], ports: [{protocol: SCTP, port: 7}]}"},
		{"only an IPv6 peer", `{FAR: [{ipBlock: {cidr: "2001:db8::/32"}}]}`},
		{"only IPv6 peers, with a port",
			`{FAR: [{ipBlock: {cidr: "2001:db8::/32", except: ["2001:db8:1::/48"]}}, {ipBlock: {cidr: "::/0"}}], ports: [{port: 80}]}`},
		{"only an IPv4-mapped peer", `{FAR: [{ipBlock: {cidr: "::ffff:10.0.0.0/104"}}]}`},
		{"both lists emptied", `{FAR: [{ipBlock: {cidr: "2001:db8::/32"}}], ports: [{protocol: SCTP, port: 7}]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			directions(t, func(t *testing.T, d model.Direction, far string) {
				dropped := strings.ReplaceAll(tt.rule, "FAR", far)
				locator := func(i int) string { return fmt.Sprintf("spec.%s[%d]", d, i) }

				s, f := git(t, ruleSpec(d, dropped))
				o := only(t, s)
				same(t, "gates", f.Gates, []model.Gate{filter(o, d, model.LabelSelector{})})
				wantProblems(t, f, problem{model.ReasonRuleOutsideModel, o.Key, locator(0)})

				// The rules around it stay, at their written places.
				s, f = git(t, ruleSpec(d, "{ports: [{port: 80}]}", dropped, "{ports: [{port: 81}]}"))
				o = only(t, s)
				want := filter(o, d, model.LabelSelector{},
					rule(o, locator(0), allPeers, []model.PortRange{tcpPorts(80, 80)}),
					rule(o, locator(2), allPeers, []model.PortRange{tcpPorts(81, 81)}))
				same(t, "gates", f.Gates, []model.Gate{want})
				wantProblems(t, f, problem{model.ReasonRuleOutsideModel, o.Key, locator(1)})
			})
		})
	}
}

func TestNetworkPolicyEveryRuleDropped(t *testing.T) {
	s, f := git(t, netpol(`
podSelector: {}
ingress:
- ports: [{protocol: SCTP, port: 7}]
egress:
- to: [{ipBlock: {cidr: "fd00::/8"}}]
`))
	o := only(t, s)
	same(t, "gates", f.Gates, []model.Gate{
		filter(o, model.Egress, model.LabelSelector{}),
		filter(o, model.Ingress, model.LabelSelector{}),
	})
	wantProblems(t, f,
		problem{model.ReasonRuleOutsideModel, o.Key, "spec.egress[0]"},
		problem{model.ReasonRuleOutsideModel, o.Key, "spec.ingress[0]"})
}

func TestNetworkPolicyMixedRule(t *testing.T) {
	directions(t, func(t *testing.T, d model.Direction, far string) {
		written := "{" + far + `: [{ipBlock: {cidr: "2001:db8::/32"}}, {ipBlock: {cidr: 10.0.0.0/8}},` +
			` {ipBlock: {cidr: "::ffff:10.0.0.0/104"}}, {podSelector: {}}],` +
			" ports: [{protocol: SCTP, port: 7}, {port: 80}, {protocol: SCTP, port: diameter}, {protocol: UDP, port: 53}]}"
		oneRule(t, d, written,
			[]model.Peer{block("10.0.0.0/8"), podPeer(nil, &model.LabelSelector{})},
			[]model.PortRange{tcpPorts(80, 80), udpPorts(53, 53)})
	})
}

// A named port alone keeps a rule whose SCTP ports are dropped.
func TestNetworkPolicyNamedPortBesideSCTP(t *testing.T) {
	oneRule(t, model.Ingress, "{ports: [{protocol: SCTP, port: 7}, {port: http}]}", allPeers, nil,
		model.NamedPort{Protocol: model.ProtocolTCP, Name: "http"})
}

func TestInvalidNetworkPolicies(t *testing.T) {
	peer := func(p string) string { return "podSelector: {}\ningress: [{from: [" + p + "]}]" }
	port := func(p string) string { return "podSelector: {}\ningress: [{ports: [" + p + "]}]" }
	selector := func(e string) string { return "podSelector: {matchExpressions: [" + e + "]}" }
	const from, ports = "spec.ingress[0].from[0]", "spec.ingress[0].ports[0]"
	for _, tt := range []struct {
		name, spec, path string
	}{
		{"cidr prefix too long", peer("{ipBlock: {cidr: 10.0.0.0/33}}"), from},
		{"cidr without a prefix length", peer("{ipBlock: {cidr: 10.0.0.1}}"), from},
		{"cidr that is no address", peer("{ipBlock: {cidr: x}}"), from},
		{"no cidr", peer("{ipBlock: {}}"), from},
		{"except outside cidr", peer("{ipBlock: {cidr: 10.0.0.0/24, except: [10.0.1.0/28]}}"), from},
		{"except wider than cidr", peer("{ipBlock: {cidr: 10.0.0.0/24, except: [10.0.0.0/16]}}"), from},
		{"except equal to cidr", peer("{ipBlock: {cidr: 10.0.0.0/24, except: [10.0.0.0/24]}}"), from},
		{"except that does not parse", peer("{ipBlock: {cidr: 10.0.0.0/24, except: [x]}}"), from},
		{"IPv6 except under IPv4", peer(`{ipBlock: {cidr: 10.0.0.0/24, except: ["2001:db8::/64"]}}`), from},
		{"endPort below port", port("{port: 100, endPort: 99}"), ports},
		{"endPort with a named port", port("{port: http, endPort: 90}"), ports},
		{"endPort without a port", port("{endPort: 90}"), ports},
		{"endPort 65536", port("{port: 1, endPort: 65536}"), ports},
		{"port 0", port("{port: 0}"), ports},
		{"port 65536", port("{port: 65536}"), ports},
		{"port name in upper case", port("{port: HTTP}"), ports},
		{"protocol in lower case", port("{protocol: tcp, port: 80}"), ports},
		{"protocol ICMP", port("{protocol: ICMP}"), ports},
		{"unknown operator", selector("{key: a, operator: Equals, values: [b]}"), "spec.podSelector"},
		{"In without values", selector("{key: a, operator: In}"), "spec.podSelector"},
		{"NotIn with empty values", selector("{key: a, operator: NotIn, values: []}"), "spec.podSelector"},
		{"Exists with values", selector("{key: a, operator: Exists, values: [b]}"), "spec.podSelector"},
		{"DoesNotExist with values", selector("{key: a, operator: DoesNotExist, values: [b]}"), "spec.podSelector"},
		{"selector label key", "podSelector: {matchLabels: {\"-x\": a}}", "spec.podSelector"},
		{"peer selector operator", peer("{podSelector: {matchExpressions: [{key: a, operator: Equals}]}}"), from},
		{"namespace selector label", peer("{namespaceSelector: {matchLabels: {a: \"b c\"}}}"), from},
		{"empty peer", peer("{}"), from},
		{"ipBlock with podSelector", peer("{ipBlock: {cidr: 10.0.0.0/8}, podSelector: {}}"), from},
		{"ipBlock with namespaceSelector", peer("{ipBlock: {cidr: 10.0.0.0/8}, namespaceSelector: {}}"), from},
		{"policyTypes in lower case", "podSelector: {}\npolicyTypes: [ingress]", "spec.policyTypes"},
		{"three policyTypes", "podSelector: {}\npolicyTypes: [Ingress, Egress, Ingress]", "spec.policyTypes"},
		{"invalid egress rule outside policyTypes",
			"podSelector: {}\npolicyTypes: [Ingress]\negress: [{ports: [{port: 0}]}]", "spec.egress[0].ports[0]"},
		{"invalid ingress rule outside policyTypes", "podSelector: {}\npolicyTypes: [Egress]\ningress: [{from: [{}]}]", from},
		{"invalid beside a dropped rule",
			"podSelector: {}\ningress: [{ports: [{protocol: SCTP}]}, {ports: [{port: 0}]}]", "spec.ingress[1].ports[0]"},
		{"invalid port of a dropped protocol", port("{protocol: SCTP, port: 0}"), ports},
		{"invalid after a valid rule",
			"podSelector: {}\ningress: [{}, {from: [{podSelector: {}}, {}]}]", "spec.ingress[1].from[1]"},
		// The first field in walk order: podSelector, ingress, egress, policyTypes.
		{"podSelector before ingress",
			"podSelector: {matchExpressions: [{key: a, operator: Equals}]}\ningress: [{from: [{}]}]", "spec.podSelector"},
		{"first of two ingress rules",
			"podSelector: {}\ningress: [{ports: [{port: 0}]}, {from: [{ipBlock: {cidr: x}}]}]", ports},
		{"ingress before egress", "podSelector: {}\negress: [{to: [{}]}]\ningress: [{from: [{}]}]", from},
		{"egress before policyTypes",
			"podSelector: {}\npolicyTypes: [ingress]\negress: [{to: [{}]}]", "spec.egress[0].to[0]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantInvalid(t, k8s.New(), netpol(tt.spec), tt.path)
		})
	}
}

// A body that does not decode into the API type is rejected as a whole, as
// strict field validation does.
func TestUndecodableNetworkPolicy(t *testing.T) {
	t.Run("unknown field", func(t *testing.T) {
		s, f := git(t, netpol("podSelector: {}\ningres: [{}]"))
		wantEmptyModel(t, f)
		wantProblems(t, f, problem{model.ReasonObjectInvalid, only(t, s).Key, ""})
	})
	for name, spec := range map[string]string{
		"ingress that is no list": "podSelector: {}\ningress: {}",
		"cidr that is no string":  "podSelector: {}\ningress: [{from: [{ipBlock: {cidr: 10}}]}]",
	} {
		t.Run(name, func(t *testing.T) {
			s, f := git(t, netpol(spec))
			wantEmptyModel(t, f)
			wantProblems(t, f, problem{model.ReasonObjectInvalid, only(t, s).Key, anyLocator})
		})
	}
}

func TestInvalidPolicyKeepsTheOthers(t *testing.T) {
	s, f := git(t, `
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: a-bad, namespace: default}
spec: {podSelector: {}, ingress: [{from: [{}]}]}
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: b-good, namespace: default}
spec: {podSelector: {}}
`)
	same(t, "gates", f.Gates, []model.Gate{filter(s.Objects[1], model.Ingress, model.LabelSelector{})})
	wantProblems(t, f, problem{model.ReasonObjectInvalid, s.Objects[0].Key, anyLocator})
}

// Expectation entries. The cases follow model.ExpectationsFile, whose
// documented example is the first one.

// expectations translates an expectations file with the given entries.
func expectations(t *testing.T, entries string) (model.Snapshot, model.Fragment) {
	t.Helper()
	doc := "apiVersion: suiro/v1\nkind: Expectations\nexpectations:\n" + entries
	s := testutil.Expectations(t, snapshot(t, ""), model.ExpectationsFileName, doc)
	return s, translate(t, k8s.New(), s)
}

// entry writes one entry named e from its fields in flow style; a field given
// as "" is left out.
func entry(sources, destinations, ports, sourceQuantifier, destinationQuantifier string) string {
	out := "- name: e\n"
	for _, field := range [][2]string{
		{"sources", sources},
		{"destinations", destinations},
		{"ports", ports},
		{"sourceQuantifier", sourceQuantifier},
		{"destinationQuantifier", destinationQuantifier},
	} {
		if field[1] != "" {
			out += "  " + field[0] + ": " + field[1] + "\n"
		}
	}
	return out
}

const (
	self    = "{self: true}"
	web     = "{internet: true}"
	onePort = "[{protocol: tcp, from: 80, to: 80}]"
)

func required(o model.Object, name string, e model.Expectation) model.Expectation {
	e.ID = string(source) + "#" + name
	e.Name = name
	e.Kind = model.ExpectRequired
	e.Mode = model.ModeWarn
	e.Origin = o.Origin
	return e
}

func TestExpectations(t *testing.T) {
	s, f := expectations(t, `
  - name: web-reaches-payments
    sources: {self: true}
    destinations: {service: payments/payments}
    ports: [{protocol: tcp, from: 80, to: 80}]
    sourceQuantifier: all
    destinationQuantifier: all
  - name: api-reaches-db
    sources: {workload: api}
    destinations: {rds: orders-db}
    ports: [{protocol: tcp, from: 5432, to: 5432}]
    sourceQuantifier: all
    destinationQuantifier: any
`)
	if len(s.Objects) != 2 {
		t.Fatalf("want two objects, got %d", len(s.Objects))
	}
	same(t, "fragment", f, model.Fragment{Source: source, Expectations: []model.Expectation{
		required(s.Objects[0], "api-reaches-db", model.Expectation{
			Sources:          model.EndpointSelector{Workload: "api"},
			Destinations:     model.EndpointSelector{RDS: "orders-db"},
			Ports:            []model.PortRange{tcpPorts(5432, 5432)},
			SourceQuantifier: model.QuantifyAll, DestinationQuantifier: model.QuantifyAny,
		}),
		required(s.Objects[1], "web-reaches-payments", model.Expectation{
			Sources:          model.EndpointSelector{Self: true},
			Destinations:     model.EndpointSelector{Service: "payments/payments"},
			Ports:            []model.PortRange{tcpPorts(80, 80)},
			SourceQuantifier: model.QuantifyAll, DestinationQuantifier: model.QuantifyAll,
		}),
	}})
}

func TestValidExpectations(t *testing.T) {
	own := model.EndpointSelector{Self: true}
	for _, tt := range []struct {
		name                  string
		sources, destinations string
		wantSources           model.EndpointSelector
		wantDestinations      model.EndpointSelector
	}{
		{"service in this namespace", self, "{service: payments}", own, model.EndpointSelector{Service: "payments"}},
		{"internet", self, web, own, model.EndpointSelector{Internet: true}},
		{"namespaces with labels", self, "{namespaces: [data], labels: {matchLabels: {app: db}}}", own,
			model.EndpointSelector{Namespaces: []string{"data"}, Labels: labels("app", "db")}},
		{"namespaces alone", self, "{namespaces: [data, shop]}", own,
			model.EndpointSelector{Namespaces: []string{"data", "shop"}}},
		{"tags", self, "{tags: {k: v}}", own, model.EndpointSelector{Tags: map[string]string{"k": "v"}}},
		{"arn", self, `{arn: "db:orders-db"}`, own, model.EndpointSelector{ARN: "db:orders-db"}},
		{"application", self, "{application: payments/api}", own, model.EndpointSelector{Application: "payments/api"}},
		{"workload with a kind", "{workload: api, workloadKind: Deployment}", web,
			model.EndpointSelector{Workload: "api", WorkloadKind: "Deployment"}, model.EndpointSelector{Internet: true}},
		{"own workloads as destinations", web, "{workload: api}",
			model.EndpointSelector{Internet: true}, model.EndpointSelector{Workload: "api"}},
		{"own workloads on both sides", self, self, own, own},
		{"several forms", "{self: true, internet: true}", "{rds: a, internet: true}",
			model.EndpointSelector{Self: true, Internet: true}, model.EndpointSelector{RDS: "a", Internet: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, f := expectations(t, entry(tt.sources, tt.destinations, onePort, "any", "all"))
			want := required(only(t, s), "e", model.Expectation{
				Sources: tt.wantSources, Destinations: tt.wantDestinations,
				Ports:            []model.PortRange{tcpPorts(80, 80)},
				SourceQuantifier: model.QuantifyAny, DestinationQuantifier: model.QuantifyAll,
			})
			same(t, "fragment", f, model.Fragment{Source: source, Expectations: []model.Expectation{want}})
		})
	}
}

func TestExpectationPorts(t *testing.T) {
	ports := "[{protocol: udp, from: 53, to: 53}, {protocol: tcp, from: 0, to: 65535}, {protocol: tcp, from: 80, to: 90}]"
	s, f := expectations(t, entry(self, web, ports, "all", "all"))
	want := required(only(t, s), "e", model.Expectation{
		Sources: model.EndpointSelector{Self: true}, Destinations: model.EndpointSelector{Internet: true},
		Ports:            []model.PortRange{udpPorts(53, 53), tcpPorts(0, 65535), tcpPorts(80, 90)},
		SourceQuantifier: model.QuantifyAll, DestinationQuantifier: model.QuantifyAll,
	})
	same(t, "fragment", f, model.Fragment{Source: source, Expectations: []model.Expectation{want}})
}

func TestInvalidExpectations(t *testing.T) {
	sources := func(s string) string { return entry(s, self, onePort, "all", "all") }
	destinations := func(d string) string { return entry(self, d, onePort, "all", "all") }
	ports := func(p string) string { return entry(self, web, p, "all", "all") }
	// A locator is exact, or with under set may also name a field inside it.
	type locator struct {
		path  string
		under bool
	}
	exact := func(path string) locator { return locator{path: path} }
	within := func(path string) locator { return locator{path: path, under: true} }

	for _, tt := range []struct {
		name  string
		entry string
		want  locator
	}{
		{"unknown selector field", sources("{self: true, wrkload: api}"), exact("")},
		{"unknown port field", ports("[{protocol: tcp, from: 80, to: 80, port: 80}]"), exact("")},
		{"port number above 65535", ports("[{protocol: tcp, from: 80, to: 65536}]"), exact("")},
		{"ports that are no list", ports("80"), exact("")},
		{"selector that is a string", sources("api"), exact("")},
		{"decoding comes first", entry("{}", "{wrkload: api}", "[]", "some", "some"), exact("")},

		{"empty sources", sources("{}"), within("sources")},
		{"no sources", entry("", self, onePort, "all", "all"), within("sources")},
		{"empty destinations", destinations("{}"), within("destinations")},
		{"no destinations", entry(self, "", onePort, "all", "all"), within("destinations")},
		{"empty tags alone", destinations("{tags: {}}"), within("destinations")},
		{"workloadKind alone", destinations("{workloadKind: Deployment}"), within("destinations")},
		{"empty namespaces alone", destinations("{namespaces: []}"), within("destinations")},
		{"no form comes before ports", entry("{}", self, "[]", "all", "all"), within("sources")},

		{"all in destinations", destinations("{all: true}"), within("destinations")},
		{"all beside self", sources("{self: true, all: true}"), within("sources")},
		{"endpoints", destinations("{endpoints: [internet]}"), within("destinations")},
		{"resourceKinds", destinations("{resourceKinds: [database]}"), within("destinations")},
		{"endpoints in sources", sources("{self: true, endpoints: [internet]}"), within("sources")},

		{"labels without namespaces", destinations("{internet: true, labels: {matchLabels: {a: b}}}"), within("destinations")},
		{"labels without namespaces in sources", sources("{self: true, labels: {matchLabels: {a: b}}}"), within("sources")},

		{"service in sources", entry("{service: payments}", self, onePort, "all", "all"), exact("sources.service")},
		{"service beside self in sources", sources("{self: true, service: payments}"), exact("sources.service")},
		{"service beside internet", destinations("{service: payments, internet: true}"), exact("destinations.service")},
		{"service beside rds", destinations("{service: payments, rds: db}"), exact("destinations.service")},
		{"service beside namespaces", destinations("{service: payments, namespaces: [a]}"), exact("destinations.service")},

		{"neither side is ours", entry("{application: a/b}", web, onePort, "all", "all"), exact("")},
		{"a service destination and foreign sources", entry(web, "{service: payments}", onePort, "all", "all"), exact("")},
		{"foreign namespaces on both sides", entry("{namespaces: [a]}", "{namespaces: [b]}", onePort, "all", "all"), exact("")},

		{"no ports", ports(""), exact("ports")},
		{"empty ports", ports("[]"), exact("ports")},

		{"protocol all", ports("[{protocol: all, from: 80, to: 80}]"), within("ports[0]")},
		{"protocol TCP", ports("[{protocol: TCP, from: 80, to: 80}]"), within("ports[0]")},
		{"protocol sctp", ports("[{protocol: sctp, from: 80, to: 80}]"), within("ports[0]")},
		{"no protocol", ports("[{from: 80, to: 80}]"), within("ports[0]")},
		{"from above to", ports("[{protocol: tcp, from: 81, to: 80}]"), within("ports[0]")},
		{"second port", ports("[{protocol: tcp, from: 80, to: 80}, {protocol: all, from: 1, to: 2}]"), within("ports[1]")},
		{"ports before quantifiers", entry(self, web, "[{protocol: all, from: 1, to: 2}]", "some", "some"), within("ports[0]")},

		{"unknown source quantifier", entry(self, web, onePort, "some", "all"), exact("sourceQuantifier")},
		{"quantifier in upper case", entry(self, web, onePort, "ALL", "all"), exact("sourceQuantifier")},
		{"no source quantifier", entry(self, web, onePort, "", "all"), exact("sourceQuantifier")},
		{"unknown destination quantifier", entry(self, web, onePort, "all", "none"), exact("destinationQuantifier")},
		{"no destination quantifier", entry(self, web, onePort, "any", ""), exact("destinationQuantifier")},
		{"source quantifier first", entry(self, web, onePort, "some", "some"), exact("sourceQuantifier")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, f := expectations(t, tt.entry)
			wantEmptyModel(t, f)
			wantProblems(t, f, problem{model.ReasonExpectationsInvalid, only(t, s).Key, anyLocator})
			got := problems(f)
			if len(got) != 1 {
				return
			}
			ok := got[0].locator == tt.want.path || tt.want.under && under(got[0].locator, tt.want.path)
			if !ok {
				t.Errorf("locator %q, want %q", got[0].locator, tt.want.path)
			}
		})
	}
}

func TestInvalidExpectationKeepsTheOthers(t *testing.T) {
	good := "    sources: {self: true}\n    destinations: {internet: true}\n" +
		"    ports: [{protocol: tcp, from: 80, to: 80}]\n    sourceQuantifier: all\n    destinationQuantifier: all\n"
	s, f := expectations(t, "  - name: a-good\n"+good+"  - name: b-bad\n    sources: {self: true}\n  - name: c-good\n"+good)
	if len(s.Objects) != 3 {
		t.Fatalf("want three objects, got %d", len(s.Objects))
	}
	var ids []string
	for _, e := range f.Expectations {
		ids = append(ids, e.ID)
	}
	if want := []string{"shop/api#a-good", "shop/api#c-good"}; !slices.Equal(ids, want) {
		t.Errorf("expectations %v, want %v", ids, want)
	}
	wantProblems(t, f, problem{model.ReasonExpectationsInvalid, s.Objects[1].Key, anyLocator})
}
