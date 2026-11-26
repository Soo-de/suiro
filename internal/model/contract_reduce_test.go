package model_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Soo-de/suiro/internal/model"
)

func jsonValue(t *testing.T, raw []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("%v in %s", err, raw)
	}
	return v
}

// reduce calls ReduceBody and checks what holds for every successful call: the
// operands are unchanged, a second call gives the same bytes, and reducing the
// result again keeps its value.
func reduce(t *testing.T, kind model.ObjectKind, body []byte, tagKeys ...string) []byte {
	t.Helper()
	raw := json.RawMessage(bytes.Clone(body))
	keys := slices.Clone(tagKeys)
	got, err := model.ReduceBody(kind, raw, keys...)
	if err != nil {
		t.Fatalf("ReduceBody(%s): %v", kind, err)
	}
	if !bytes.Equal(raw, body) {
		t.Errorf("ReduceBody changed its body to %s", raw)
	}
	if !slices.Equal(keys, tagKeys) {
		t.Errorf("ReduceBody changed its tag keys to %q", keys)
	}
	got = bytes.Clone(got)
	for range 5 {
		again, err := model.ReduceBody(kind, raw, keys...)
		if err != nil {
			t.Fatalf("ReduceBody(%s), a later call: %v", kind, err)
		}
		if !bytes.Equal(again, got) {
			t.Fatalf("calls differ:\n%s\n%s", got, again)
		}
	}
	twice, err := model.ReduceBody(kind, got, tagKeys...)
	if err != nil {
		t.Fatalf("ReduceBody(%s) of its own result %s: %v", kind, got, err)
	}
	if !reflect.DeepEqual(jsonValue(t, twice), jsonValue(t, got)) {
		t.Errorf("not idempotent:\nonce  %s\ntwice %s", got, twice)
	}
	return got
}

func checkReduced(t *testing.T, kind model.ObjectKind, in, want []byte, tagKeys ...string) {
	t.Helper()
	got := reduce(t, kind, in, tagKeys...)
	if !reflect.DeepEqual(jsonValue(t, got), jsonValue(t, want)) {
		t.Errorf("ReduceBody(%s) =\n%s\nwant\n%s", kind, got, want)
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "reduce", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The inputs have the shapes of the Kubernetes documentation's examples and of
// the AWS Describe* output types; every value to be dropped holds SENTINEL, an
// account ID or an ARN.
func TestReduceBodyFixtures(t *testing.T) {
	tests := []struct {
		name    string
		kind    model.ObjectKind
		tagKeys []string
	}{
		{"deployment", model.KindDeployment, nil},
		{"cronjob", model.KindCronJob, nil},
		{"vpc", model.KindVPC, nil},
		{"subnet", model.KindSubnet, nil},
		{"securitygroup", model.KindSecurityGroup, nil},
		{"networkacl", model.KindNetworkACL, nil},
		{"routetable", model.KindRouteTable, nil},
		{"networkinterface", model.KindNetworkInterface, nil},
		{"natgateway", model.KindNATGateway, nil},
		{"instance", model.KindInstance, nil},
		{"dbinstance", model.KindDBInstance, []string{"team"}},
		{"ekscluster", model.KindEKSCluster, nil},
		{"nodegroup", model.KindNodegroup, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, want := fixture(t, tt.name+".in.json"), fixture(t, tt.name+".want.json")
			checkReduced(t, tt.kind, in, want, tt.tagKeys...)
			got := reduce(t, tt.kind, in, tt.tagKeys...)
			for _, secret := range []string{"SENTINEL", "123456789012", "arn:aws"} {
				if bytes.Contains(got, []byte(secret)) {
					t.Errorf("the result holds %q: %s", secret, got)
				}
			}
		})
	}
}

// Every controller with a template at spec.template keeps the same fields.
func TestReduceBodyPodTemplateKinds(t *testing.T) {
	kinds := []model.ObjectKind{
		model.KindDeployment, model.KindStatefulSet, model.KindDaemonSet, model.KindReplicaSet, model.KindJob,
		// A custom kind with a pod template keeps it too.
		"k8s:example.com/Rollout",
	}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			checkReduced(t, kind, fixture(t, "deployment.in.json"), fixture(t, "deployment.want.json"))
		})
	}
}

func TestReduceBodyKubernetes(t *testing.T) {
	const identity = `{"apiVersion":"v1","kind":"X","metadata":{"name":"n","namespace":"ns"}}`
	tests := []struct {
		name     string
		kind     model.ObjectKind
		in, want string
	}{
		{
			"secret keeps identity only", "k8s:core/Secret",
			`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"s","namespace":"ns","labels":{"a":"b"},"annotations":{"x":"y"}},
			  "type":"Opaque","data":{"password":"c2VjcmV0"},"stringData":{"token":"t"}}`,
			`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"s","namespace":"ns"}}`,
		},
		{
			"config map keeps identity only", "k8s:core/ConfigMap",
			`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"c","labels":{"a":"b"}},"data":{"k":"v"},"binaryData":{"b":"AA=="}}`,
			`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"c"}}`,
		},
		{
			"custom kind without a pod template", "k8s:example.com/Widget",
			`{"apiVersion":"v1","kind":"X","metadata":{"name":"n","namespace":"ns","labels":{"a":"b"}},"spec":{"template":{"spec":{"nodeName":"n1"}},"size":3}}`,
			identity,
		},
		{
			"custom kind without a spec", "k8s:example.com/Widget",
			`{"apiVersion":"v1","kind":"X","metadata":{"name":"n","namespace":"ns"},"data":{"k":"v"}}`,
			identity,
		},
		{
			"unsupported kind keeps identity only", "k8s:cilium.io/CiliumNetworkPolicy",
			`{"apiVersion":"v1","kind":"X","metadata":{"name":"n","namespace":"ns"},"spec":{"endpointSelector":{}}}`,
			identity,
		},
		{
			"namespace keeps labels", model.KindNamespace,
			`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"payments","labels":{"team":"payments"},"annotations":{"a":"b"}},"spec":{"finalizers":["kubernetes"]}}`,
			`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"payments","labels":{"team":"payments"}}}`,
		},
		{
			"pod", model.KindPod,
			`{"apiVersion":"v1","kind":"Pod","metadata":{"name":"p","labels":{"app":"p"},"annotations":{"a":"b"}},
			  "spec":{"hostNetwork":true,"nodeName":"n1","nodeSelector":{"a":"b"},"affinity":{},"tolerations":[],"serviceAccountName":"sa",
			    "containers":[{"name":"c","image":"i","env":[{"name":"E","value":"v"}],"ports":[{"containerPort":80}]},{"name":"d","args":["x"]}]},
			  "status":{"podIP":"10.0.0.1"}}`,
			`{"apiVersion":"v1","kind":"Pod","metadata":{"name":"p","labels":{"app":"p"}},
			  "spec":{"hostNetwork":true,"nodeName":"n1","nodeSelector":{"a":"b"},"affinity":{},"tolerations":[],
			    "containers":[{"name":"c","ports":[{"containerPort":80}]},{"name":"d"}]}}`,
		},
		{
			// The NetworkPolicy "default-deny-ingress" shape of the
			// documentation, with an empty ingress list.
			"network policy keeps the whole spec", model.KindNetworkPolicy,
			`{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":"test","namespace":"default","labels":{"a":"b"},"annotations":{"a":"b"}},
			  "spec":{"podSelector":{},"policyTypes":["Ingress","Egress"],"ingress":[],
			    "egress":[{"to":[{"ipBlock":{"cidr":"10.0.0.0/24","except":["10.0.0.1/32"]}}],"ports":[{"protocol":"TCP","port":5978,"endPort":5999}]},{}]}}`,
			`{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":"test","namespace":"default"},
			  "spec":{"podSelector":{},"policyTypes":["Ingress","Egress"],"ingress":[],
			    "egress":[{"to":[{"ipBlock":{"cidr":"10.0.0.0/24","except":["10.0.0.1/32"]}}],"ports":[{"protocol":"TCP","port":5978,"endPort":5999}]},{}]}}`,
		},
		{
			"service", model.KindService,
			`{"apiVersion":"v1","kind":"Service","metadata":{"name":"my-service","labels":{"a":"b"}},
			  "spec":{"type":"ClusterIP","clusterIP":"10.0.171.239","clusterIPs":["10.0.171.239"],"externalIPs":["198.51.100.1"],
			    "selector":{"app.kubernetes.io/name":"MyApp"},"ports":[{"protocol":"TCP","port":80,"targetPort":"http"}]}}`,
			`{"apiVersion":"v1","kind":"Service","metadata":{"name":"my-service"},
			  "spec":{"type":"ClusterIP","selector":{"app.kubernetes.io/name":"MyApp"},"ports":[{"protocol":"TCP","port":80,"targetPort":"http"}]}}`,
		},
		{
			"ingress", model.KindIngress,
			`{"apiVersion":"networking.k8s.io/v1","kind":"Ingress","metadata":{"name":"i","annotations":{"a":"b"}},"spec":{"ingressClassName":"nginx","rules":[{"host":"h"}]}}`,
			`{"apiVersion":"networking.k8s.io/v1","kind":"Ingress","metadata":{"name":"i"},"spec":{"ingressClassName":"nginx"}}`,
		},
		{
			"missing kept fields stay absent", model.KindDeployment,
			`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"d"},"spec":{"template":{"spec":{"containers":[{"name":"c"}]}}}}`,
			`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"d"},"spec":{"template":{"spec":{"containers":[{"name":"c"}]}}}}`,
		},
		{
			"service with a selector only", model.KindService,
			`{"apiVersion":"v1","kind":"Service","metadata":{"name":"s"},"spec":{"selector":{"a":"b"}}}`,
			`{"apiVersion":"v1","kind":"Service","metadata":{"name":"s"},"spec":{"selector":{"a":"b"}}}`,
		},
		{
			"kept null and empty values stay", model.KindService,
			`{"apiVersion":"v1","kind":"Service","metadata":{"name":"s"},"spec":{"selector":{},"ports":[],"type":null}}`,
			`{"apiVersion":"v1","kind":"Service","metadata":{"name":"s"},"spec":{"selector":{},"ports":[],"type":null}}`,
		},
		{
			"expectation drops what a writer adds", model.KindExpectation,
			`{"name":"api-reaches-db","apiVersion":"x","note":"extra","sources":{"workload":"api"},"destinations":{"rds":"orders-db"},
			  "ports":[{"protocol":"tcp","from":5432,"to":5432}],"sourceQuantifier":"all","destinationQuantifier":"any"}`,
			`{"name":"api-reaches-db","sources":{"workload":"api"},"destinations":{"rds":"orders-db"},
			  "ports":[{"protocol":"tcp","from":5432,"to":5432}],"sourceQuantifier":"all","destinationQuantifier":"any"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkReduced(t, tt.kind, []byte(tt.in), []byte(tt.want))
		})
	}
}

// The forms are the ones FieldPath lists, as AWS writes them for the
// interfaces its load balancers and RDS manage.
func TestReduceBodyNetworkInterfaceDescription(t *testing.T) {
	name32, name33 := strings.Repeat("a", 32), strings.Repeat("a", 33)
	tests := []struct {
		description string
		kept        bool
	}{
		{"ELB app/my-load-balancer/50dc6c495c0c9188", true},
		{"ELB net/x/0123456789abcdef", true},
		{"ELB my-clb", true},
		{"RDSNetworkInterface", true},
		{"ELB " + name32, true},
		{"ELB app/" + name32 + "/0123456789abcdef", true},
		{"arn:aws:ecs:eu-central-1:123456789012:attachment/0f1e2d3c-1111-2222-3333-444455556666", false},
		{"free text written by someone", false},
		{"", false},
		{"ELB ", false},
		{"ELB " + name33, false},
		{"ELB app/" + name33 + "/0123456789abcdef", false},
		{"ELB net/" + name33 + "/0123456789abcdef", false},
		{"ELB app//0123456789abcdef", false},
		{"ELB app/x/0123456789abcde", false},
		{"ELB app/x/0123456789abcdef0", false},
		{"ELB app/x/0123456789abcdeg", false},
		{"ELB app/x", false},
		{"ELB gwy/x/0123456789abcdef", false},
		{"ELB my_clb", false},
		{"ELB my clb", false},
		{"ELB app/x/0123456789abcdef extra", false},
		{"my ELB my-clb", false},
		{"ELB my-clb\nsecret", false},
		{"secret\nRDSNetworkInterface", false},
		{"RDSNetworkInterface for orders", false},
		{"elb my-clb", false},
	}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			in := mustJSON(t, map[string]any{"NetworkInterfaceId": "eni-0abc", "Description": tt.description})
			want := map[string]any{"NetworkInterfaceId": "eni-0abc"}
			if tt.kept {
				want["Description"] = tt.description
			}
			checkReduced(t, model.KindNetworkInterface, in, mustJSON(t, want))
		})
	}
}

type tag struct{ Key, Value string }

// tagsOf returns the entries of the tag list field of a reduced body; an
// absent list and an empty one both hold none.
func tagsOf(t *testing.T, body []byte, field string) []tag {
	t.Helper()
	var v map[string]json.RawMessage
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatal(err)
	}
	var tags []tag
	if raw, ok := v[field]; ok {
		if err := json.Unmarshal(raw, &tags); err != nil {
			t.Fatal(err)
		}
	}
	return tags
}

func TestReduceBodyTagKeys(t *testing.T) {
	var (
		group   = tag{"eks:nodegroup-name", "workers"}
		cluster = tag{"eks:cluster-name", "prod"}
		team    = tag{"team", "payments"}
		dotted  = tag{"app.kubernetes.io/name", "api"}
		secret  = tag{"secret", "s"}
		caps    = tag{"Team", "other"}
	)
	tests := []struct {
		name    string
		kind    model.ObjectKind
		field   string
		in      []tag
		tagKeys []string
		want    []tag
	}{
		{"instance with a tag key", model.KindInstance, "Tags", []tag{group, cluster, team, secret}, []string{"team"}, []tag{group, cluster, team}},
		{"instance without tag keys", model.KindInstance, "Tags", []tag{group, cluster, team, secret}, nil, []tag{group, cluster}},
		{"instance keeps source order", model.KindInstance, "Tags", []tag{team, secret, cluster, group}, []string{"team"}, []tag{team, cluster, group}},
		{"instance with two tag keys", model.KindInstance, "Tags", []tag{dotted, secret, team}, []string{"team", "app.kubernetes.io/name"}, []tag{dotted, team}},
		{"instance with an absent tag key", model.KindInstance, "Tags", []tag{secret, group}, []string{"team"}, []tag{group}},
		{"tag keys compare exactly", model.KindInstance, "Tags", []tag{caps, team}, []string{"team"}, []tag{team}},
		{"tag key naming a kept tag", model.KindInstance, "Tags", []tag{group, secret}, []string{"eks:nodegroup-name"}, []tag{group}},
		{"database with a tag key", model.KindDBInstance, "TagList", []tag{secret, team, group}, []string{"team"}, []tag{team}},
		{"database without tag keys", model.KindDBInstance, "TagList", []tag{team, group, cluster}, nil, nil},
		{"security group ignores tag keys", model.KindSecurityGroup, "Tags", []tag{team, group}, []string{"team"}, nil},
		{"subnet ignores tag keys", model.KindSubnet, "Tags", []tag{team}, []string{"team"}, nil},
		{"database ignores Tags", model.KindDBInstance, "Tags", []tag{team}, []string{"team"}, nil},
		{"instance ignores TagList", model.KindInstance, "TagList", []tag{team}, []string{"team"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := mustJSON(t, map[string]any{tt.field: tt.in})
			got := reduce(t, tt.kind, in, tt.tagKeys...)
			if tags := tagsOf(t, got, tt.field); !slices.Equal(tags, tt.want) {
				t.Errorf("%s = %v, want %v (%s)", tt.field, tags, tt.want, got)
			}
		})
	}
}

func TestReduceBodyAddonConfiguration(t *testing.T) {
	const asJSON = `{"enableNetworkPolicy":"true","resources":{"limits":{"cpu":"SENTINEL-cpu"}},` +
		`"env":{"NETWORK_POLICY_ENFORCING_MODE":"strict","AWS_VPC_K8S_CNI_EXTERNALSNAT":"false",` +
		`"AWS_VPC_K8S_CNI_EXCLUDE_SNAT_CIDRS":"10.1.0.0/16","ENABLE_POD_ENI":"true",` +
		`"AWS_VPC_K8S_CNI_CUSTOM_NETWORK_CFG":"false","ENABLE_PREFIX_DELEGATION":"true","SECRET_TOKEN":"SENTINEL-token"}}`
	const asYAML = `enableNetworkPolicy: "true"
resources:
  limits:
    cpu: SENTINEL-cpu
env:
  NETWORK_POLICY_ENFORCING_MODE: strict
  AWS_VPC_K8S_CNI_EXTERNALSNAT: "false"
  AWS_VPC_K8S_CNI_EXCLUDE_SNAT_CIDRS: 10.1.0.0/16
  ENABLE_POD_ENI: "true"
  AWS_VPC_K8S_CNI_CUSTOM_NETWORK_CFG: "false"
  ENABLE_PREFIX_DELEGATION: "true"
  SECRET_TOKEN: SENTINEL-token
`
	const kept = `{"enableNetworkPolicy":"true","env":{"NETWORK_POLICY_ENFORCING_MODE":"strict",` +
		`"AWS_VPC_K8S_CNI_EXTERNALSNAT":"false","AWS_VPC_K8S_CNI_EXCLUDE_SNAT_CIDRS":"10.1.0.0/16",` +
		`"ENABLE_POD_ENI":"true","AWS_VPC_K8S_CNI_CUSTOM_NETWORK_CFG":"false","ENABLE_PREFIX_DELEGATION":"true"}}`
	for _, tt := range []struct{ name, configuration string }{{"JSON string", asJSON}, {"YAML string", asYAML}} {
		t.Run(tt.name, func(t *testing.T) {
			in := mustJSON(t, map[string]any{
				"AddonName": "vpc-cni", "AddonVersion": "v1.19.0",
				"AddonArn":            "arn:aws:eks:eu-central-1:123456789012:addon/prod/vpc-cni/x",
				"ConfigurationValues": tt.configuration,
			})
			want := `{"AddonName":"vpc-cni","AddonVersion":"v1.19.0","ConfigurationValues":` + kept + `}`
			checkReduced(t, model.KindEKSAddon, in, []byte(want))
		})
	}
	t.Run("absent", func(t *testing.T) {
		const in = `{"AddonName":"vpc-cni","AddonVersion":"v1.19.0"}`
		checkReduced(t, model.KindEKSAddon, []byte(in), []byte(in))
	})
}

func TestReduceBodyErrors(t *testing.T) {
	tests := []struct {
		name string
		kind model.ObjectKind
		in   string
	}{
		{"array", model.KindDeployment, `[]`},
		{"string", model.KindDeployment, `"x"`},
		{"number", model.KindDeployment, `1`},
		{"null", model.KindDeployment, `null`},
		{"empty", model.KindDeployment, ``},
		{"truncated", model.KindDeployment, `{"a":`},
		{"two values", model.KindDeployment, `{} {}`},
		{"array of an AWS kind", model.KindSecurityGroup, `[]`},
		{"array of a custom kind", "k8s:example.com/Widget", `[]`},
		{"AWS kind without an entry", "aws:ec2/VpcEndpoint", `{}`},
		{"other prefix without an entry", "gcp:compute/Firewall", `{}`},
		{"suiro kind without an entry", "suiro:core/Other", `{}`},
		{"no prefix", "core/Service", `{}`},
		{"empty kind", "", `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := model.ReduceBody(tt.kind, json.RawMessage(tt.in)); err == nil {
				t.Errorf("ReduceBody(%q, %s) = %s, want an error", tt.kind, tt.in, got)
			}
		})
	}
}

func TestReduceBodyNumberTooLarge(t *testing.T) {
	tests := []struct {
		name     string
		kind     model.ObjectKind
		in       string
		tagKeys  []string
		tooLarge bool
	}{
		{"kept port", model.KindSecurityGroup, `{"IpPermissions":[{"FromPort":443},{"FromPort":9007199254740993}]}`, nil, true},
		{"negative", model.KindSecurityGroup, `{"IpPermissions":[{"ToPort":-9007199254740993}]}`, nil, true},
		{"huge exponent", model.KindSecurityGroup, `{"IpPermissions":[{"ToPort":1e400}]}`, nil, true},
		{"deep in a kept spec", model.KindNetworkPolicy, `{"spec":{"ingress":[{"ports":[{"port":9007199254740993}]}]}}`, nil, true},
		{"identity field", "k8s:example.com/Widget", `{"metadata":{"name":9007199254740993}}`, nil, true},
		{"pod template of a custom kind", "k8s:example.com/Widget", `{"spec":{"template":{"spec":{"containers":[{"ports":[{"containerPort":1e16}]}]}}}}`, nil, true},
		{"largest kept number", model.KindSecurityGroup, `{"IpPermissions":[{"FromPort":9007199254740992,"ToPort":-9007199254740992}]}`, nil, false},
		{"dropped field", model.KindDeployment, `{"spec":{"replicas":9007199254740993}}`, nil, false},
		{"dropped list field", model.KindSecurityGroup, `{"IpPermissions":[{"FromPort":1,"Extra":9007199254740993}]}`, nil, false},
		{"dropped tag", model.KindInstance, `{"Tags":[{"Key":"secret","Value":"s","Extra":9007199254740993}]}`, nil, false},
		{"string", model.KindSecurityGroup, `{"GroupId":"9007199254740993"}`, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := model.ReduceBody(tt.kind, json.RawMessage(tt.in), tt.tagKeys...)
			if tt.tooLarge {
				if !errors.Is(err, model.ErrNumberTooLarge) {
					t.Errorf("ReduceBody = %s, %v; want ErrNumberTooLarge", got, err)
				}
				return
			}
			if err != nil {
				t.Errorf("ReduceBody: %v", err)
			}
		})
	}
}

// The largest exact integer survives as written.
func TestReduceBodyKeepsLargeInteger(t *testing.T) {
	got := reduce(t, model.KindSecurityGroup, []byte(`{"IpPermissions":[{"FromPort":9007199254740992}]}`))
	dec := json.NewDecoder(bytes.NewReader(got))
	dec.UseNumber()
	var v struct {
		IpPermissions []struct{ FromPort json.Number }
	}
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	if len(v.IpPermissions) != 1 {
		t.Fatalf("got %s", got)
	}
	if n, err := v.IpPermissions[0].FromPort.Float64(); err != nil || n != 1<<53 {
		t.Errorf("FromPort = %s, want 9007199254740992", v.IpPermissions[0].FromPort)
	}
}
