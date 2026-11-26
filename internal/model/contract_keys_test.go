package model_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/Soo-de/suiro/internal/model"
)

// The expected keys are the examples of snapshot.go and gate.go.
func TestObjectKeyExamples(t *testing.T) {
	tests := []struct {
		name      string
		kind      model.ObjectKind
		namespace string
		object    string
		want      model.ObjectKey
	}{
		{"namespaced deployment", model.KindDeployment, "payments", "api", "k8s:apps/Deployment/payments/api"},
		{"security group", model.KindSecurityGroup, "", "sg-0123", "aws:ec2/SecurityGroup/sg-0123"},
		{"network policy", model.KindNetworkPolicy, "payments", "allow-db", "k8s:networking.k8s.io/NetworkPolicy/payments/allow-db"},
		{"cluster-scoped namespace", model.KindNamespace, "", "payments", "k8s:core/Namespace/payments"},
		{"dotted name", model.KindService, "kube-system", "kube-root-ca.crt", "k8s:core/Service/kube-system/kube-root-ca.crt"},
		{"expectation", model.KindExpectation, "", "payments/payments-api#api-reaches-db", "suiro:core/Expectation/payments/payments-api#api-reaches-db"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.NewObjectKey(tt.kind, tt.namespace, tt.object)
			if got != tt.want {
				t.Fatalf("NewObjectKey(%q, %q, %q) = %q, want %q", tt.kind, tt.namespace, tt.object, got, tt.want)
			}
			if again := model.NewObjectKey(tt.kind, tt.namespace, tt.object); again != got {
				t.Errorf("a second call gave %q, the first %q", again, got)
			}
			kind, ns, name, err := tt.want.Parse()
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.want, err)
			}
			if kind != tt.kind || ns != tt.namespace || name != tt.object {
				t.Errorf("Parse(%q) = (%q, %q, %q), want (%q, %q, %q)", tt.want, kind, ns, name, tt.kind, tt.namespace, tt.object)
			}
		})
	}
}

// Names follow the Kubernetes rules: a namespace is a DNS-1123 label, an
// object name a DNS-1123 subdomain.
const (
	dnsLabel     = `[a-z0-9]([-a-z0-9]{0,10}[a-z0-9])?`
	dnsSubdomain = dnsLabel + `(\.` + dnsLabel + `){0,3}`
)

func TestObjectKeyRoundTrip(t *testing.T) {
	kinds := append(model.KeptKinds(), "k8s:example.com/Widget")
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			rapid.Check(t, func(t *rapid.T) {
				var namespace, name string
				if kind == model.KindExpectation {
					// An Expectation.ID is "<team>/<app>#<name>" and has no namespace.
					name = rapid.StringMatching(dnsLabel+`/`+dnsLabel+`#`+dnsLabel).Draw(t, "id")
				} else {
					namespace = rapid.OneOf(rapid.Just(""), rapid.StringMatching(dnsLabel)).Draw(t, "namespace")
					name = rapid.StringMatching(dnsSubdomain).Draw(t, "name")
				}
				key := model.NewObjectKey(kind, namespace, name)
				gotKind, gotNS, gotName, err := key.Parse()
				if err != nil {
					t.Fatalf("Parse(%q): %v", key, err)
				}
				if gotKind != kind || gotNS != namespace || gotName != name {
					t.Fatalf("Parse(%q) = (%q, %q, %q), want (%q, %q, %q)", key, gotKind, gotNS, gotName, kind, namespace, name)
				}
			})
		})
	}
}

func TestObjectKeyParseErrors(t *testing.T) {
	tests := []model.ObjectKey{
		"",
		"k8s:core/Service",
		"k8s:core/Service/",
		"k8s:core/Service/a/b/c",
		"k8s:core/Service//x",
		"k8s:core/Service/ns/",
		"suiro:core/Expectation/",
		"core/Service/x",
		":core/Service/x",
	}
	for _, key := range tests {
		t.Run(string(key), func(t *testing.T) {
			if kind, ns, name, err := key.Parse(); err == nil {
				t.Errorf("Parse(%q) = (%q, %q, %q), want an error", key, kind, ns, name)
			}
		})
	}
}

func TestGateIDFilter(t *testing.T) {
	tests := []struct {
		object    model.ObjectKey
		direction model.Direction
		want      model.GateID
	}{
		{"aws:ec2/SecurityGroup/sg-0abc", model.Ingress, "aws:ec2/SecurityGroup/sg-0abc#ingress"},
		{"aws:ec2/SecurityGroup/sg-0abc", model.Egress, "aws:ec2/SecurityGroup/sg-0abc#egress"},
		{"k8s:networking.k8s.io/NetworkPolicy/payments/allow-db", model.Egress, "k8s:networking.k8s.io/NetworkPolicy/payments/allow-db#egress"},
	}
	for _, tt := range tests {
		t.Run(string(tt.want), func(t *testing.T) {
			for range 2 {
				if got := model.NewGateID(tt.object, tt.direction); got != tt.want {
					t.Errorf("NewGateID(%q, %q) = %q, want %q", tt.object, tt.direction, got, tt.want)
				}
			}
		})
	}
}

func TestPathKeyString(t *testing.T) {
	const (
		api model.EndpointKey = "k8s:apps/Deployment/payments/api"
		db  model.EndpointKey = "aws:database/orders-db"
	)
	tests := []struct {
		name string
		key  model.PathKey
		want string
	}{
		{
			"single port",
			model.PathKey{Source: api, Destination: db, Ports: model.PortRange{Protocol: model.ProtocolTCP, From: 5432, To: 5432}},
			"k8s:apps/Deployment/payments/api → aws:database/orders-db : tcp/5432",
		},
		{
			"all udp ports",
			model.PathKey{Source: api, Destination: db, Ports: model.PortRange{Protocol: model.ProtocolUDP, From: 0, To: 65535}},
			"k8s:apps/Deployment/payments/api → aws:database/orders-db : udp/0-65535",
		},
		{
			"port zero",
			model.PathKey{Source: model.EndpointInternet, Destination: api, Ports: model.PortRange{Protocol: model.ProtocolTCP}},
			"internet → k8s:apps/Deployment/payments/api : tcp/0",
		},
		{
			"named network and range",
			model.PathKey{Source: "external-private:office-vpn", Destination: api, Ports: model.PortRange{Protocol: model.ProtocolTCP, From: 80, To: 81}},
			"external-private:office-vpn → k8s:apps/Deployment/payments/api : tcp/80-81",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.key
			if got := tt.key.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
			if tt.key != before {
				t.Errorf("String changed the key to %+v", tt.key)
			}
			got, err := model.ParsePathKey(tt.want)
			if err != nil {
				t.Fatalf("ParsePathKey(%q): %v", tt.want, err)
			}
			if got != tt.key {
				t.Errorf("ParsePathKey(%q) = %+v, want %+v", tt.want, got, tt.key)
			}
		})
	}
}

func TestPathKeyRoundTrip(t *testing.T) {
	endpoint := rapid.Custom(func(t *rapid.T) model.EndpointKey {
		return model.EndpointKey(rapid.OneOf(
			rapid.StringMatching(`k8s:apps/Deployment/`+dnsLabel+`/`+dnsSubdomain),
			rapid.StringMatching(`aws:database/`+dnsLabel),
			rapid.SampledFrom([]string{"internet", "external-private", "link-local"}),
			rapid.StringMatching(`(external-private|link-local):`+dnsLabel),
		).Draw(t, "endpoint"))
	})
	rapid.Check(t, func(t *rapid.T) {
		from := rapid.Uint16().Draw(t, "from")
		to := from
		if rapid.Bool().Draw(t, "range") {
			to = rapid.Uint16Range(from, 65535).Draw(t, "to")
		}
		key := model.PathKey{
			Source:      endpoint.Draw(t, "source"),
			Destination: endpoint.Draw(t, "destination"),
			Ports: model.PortRange{
				Protocol: rapid.SampledFrom([]model.Protocol{model.ProtocolTCP, model.ProtocolUDP}).Draw(t, "protocol"),
				From:     from,
				To:       to,
			},
		}
		s := key.String()
		got, err := model.ParsePathKey(s)
		if err != nil {
			t.Fatalf("ParsePathKey(%q): %v", s, err)
		}
		if got != key {
			t.Fatalf("ParsePathKey(%q) = %+v, want %+v", s, got, key)
		}
	})
}

func TestPathKeyParseErrors(t *testing.T) {
	// Each case breaks "a → b : tcp/80", which is valid, in one way.
	const valid = "a → b : tcp/80"
	if _, err := model.ParsePathKey(valid); err != nil {
		t.Fatalf("ParsePathKey(%q): %v", valid, err)
	}
	tests := []struct{ name, in string }{
		{"empty", ""},
		{"range of one port", "a → b : tcp/5432-5432"},
		{"leading zero", "a → b : tcp/05432"},
		{"leading zero in the end", "a → b : tcp/80-081"},
		{"port too large", "a → b : tcp/65536"},
		{"range end too large", "a → b : tcp/0-65536"},
		{"reversed range", "a → b : tcp/10-5"},
		{"upper-case protocol", "a → b : TCP/80"},
		{"sctp", "a → b : sctp/80"},
		{"all protocols", "a → b : all/80"},
		{"ascii arrow", "a -> b : tcp/80"},
		{"doubled space before the arrow", "a  → b : tcp/80"},
		{"doubled space after the arrow", "a →  b : tcp/80"},
		{"doubled space before the colon", "a → b  : tcp/80"},
		{"doubled space after the colon", "a → b :  tcp/80"},
		{"empty source", " → b : tcp/80"},
		{"empty destination", "a →  : tcp/80"},
		{"source with a space", "a c → b : tcp/80"},
		{"destination with a space", "a → b c : tcp/80"},
		{"no port", "a → b : tcp/"},
		{"no protocol", "a → b : 80"},
		{"no ports part", "a → b"},
		{"signed port", "a → b : tcp/+5"},
		{"negative port", "a → b : tcp/-5"},
		{"open range", "a → b : tcp/5-"},
		{"three-part range", "a → b : tcp/1-2-3"},
		{"trailing text", "a → b : tcp/80 x"},
		{"trailing space", "a → b : tcp/80 "},
		{"leading space", " a → b : tcp/80"},
		{"trailing newline", "a → b : tcp/80\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := model.ParsePathKey(tt.in); err == nil {
				t.Errorf("ParsePathKey(%q) = %+v, want an error", tt.in, got)
			}
		})
	}
}
