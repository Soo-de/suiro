package model_test

import (
	"errors"
	"net/netip"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Soo-de/suiro/internal/model"
)

const aboveExact = 1<<53 + 1

// fingerprintBundle is a fresh pull request Bundle with every part filled and
// at least two entries in each list.
func fingerprintBundle() model.Bundle {
	prefix := netip.MustParsePrefix
	return model.Bundle{
		Kind:        model.AnalysisPullRequest,
		Environment: "prod",
		PullRequest: &model.PullRequest{
			RepoURL: "https://github.com/acme/payments", Number: 7,
			HeadSHA: "head", MergeSHA: "merge", TargetBranch: "main",
		},
		Sources: []model.Source{
			{
				ID: "aws/prod", Kind: model.SourceAWS, Environment: "prod",
				AWS:         &model.AWSSource{Region: "eu-central-1", VPCID: "vpc-0abc", ClusterName: "prod"},
				KeptTagKeys: []string{"app", "team"},
			},
			{ID: "cluster/prod", Kind: model.SourceCluster, Environment: "prod", Cluster: &model.ClusterSource{}},
			{
				ID: "payments/api", Kind: model.SourceGit, Environment: "prod",
				Owner: model.Owner{Team: "payments", Application: "api"},
				Git: &model.GitSource{
					RepoURL: "https://github.com/acme/payments", Path: "deploy", TargetRevision: "main",
					Render: model.RenderHelm, Namespace: "payments",
					Helm: &model.HelmSource{
						ReleaseName: "api",
						ValueFiles:  []string{"values.yaml", "values-prod.yaml"},
						Parameters:  []model.HelmParameter{{Name: "a", Value: "1"}, {Name: "b", Value: "2"}},
					},
				},
			},
		},
		Snapshots: []model.Snapshot{
			{
				Key: model.ReadKey{Source: "aws/prod"}, ContractVersion: 1, Digest: sha("aws"),
				ReadStartedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				Objects:       []model.Object{},
			},
			{
				Key: model.ReadKey{Source: "payments/api", PullRequest: 7}, Revision: "merge", ContractVersion: 1, Digest: sha("git"),
				Objects:  []model.Object{object("k8s:core/ConfigMap/payments/a", `{"a":1}`)},
				Problems: []model.Problem{{Reason: model.ReasonKindUnrecognized, Subject: "x"}},
			},
		},
		UsedNamespaces: map[model.SourceID][]string{"payments/api": {"jobs", "payments"}},
		Problems: []model.Problem{
			{Reason: model.ReasonSourceStale, Subject: "aws/prod", Message: "m", Origin: &model.Origin{Source: "aws/prod"}},
			{Reason: model.ReasonSourceStale, Subject: "payments/api"},
			{Reason: model.ReasonSourceUnread, Subject: "cluster/prod"},
		},
		Profile: model.ClusterProfile{
			CNI:                      "vpc-cni",
			NetworkPolicyEnforcement: model.ToggleOn,
			NetworkPolicyMode:        "standard",
			ExternalSNAT:             model.ToggleOff,
			ExcludeSNATCIDRs:         []netip.Prefix{prefix("10.1.0.0/16")},
			EphemeralPorts:           &model.PortRange{Protocol: model.ProtocolTCP, From: 1024, To: 65535},
			NodeGroups: []model.NodeGroup{{
				Name: "workers", Subnets: []string{"subnet-a"}, SecurityGroups: []string{"sg-0abc"},
			}},
			PodCIDRs:          []netip.Prefix{prefix("10.244.0.0/16")},
			PrivateNetworks:   []model.NamedNetwork{{Name: "vpn", Prefixes: []netip.Prefix{prefix("10.9.0.0/16")}}},
			LinkLocalNetworks: []model.NamedNetwork{{Name: "imds", Prefixes: []netip.Prefix{prefix("169.254.169.254/32")}}},
			NamespaceLabels:   map[string]map[string]string{"payments": {"team": "payments"}},
			KeptTagKeys:       []string{"app", "team"},
		},
		Rules: model.RuleSet{
			Version:   3,
			Forbidden: []model.Expectation{{ID: "builtin.a", Kind: model.ExpectForbidden}},
			Hygiene:   []model.HygieneRule{{ID: "h", Check: model.CheckHostNetworkPod}},
		},
		Before:           &model.Analysis{ID: 4, Fingerprint: sha("before"), ToolVersion: "v0.1.0", Paths: []model.Path{}},
		Fingerprint:      sha("stale"),
		ToolVersion:      "v0.2.0",
		SchemaVersion:    model.SchemaVersion,
		AlgorithmVersion: model.AlgorithmVersion,
	}
}

func baselineBundle() model.Bundle {
	b := fingerprintBundle()
	b.Kind = model.AnalysisBaseline
	b.PullRequest = nil
	return b
}

func fingerprintLimits() model.Limits {
	return model.Limits{Endpoints: 1000, Rules: 2000, PrefixesPerEndpoint: 30, PortRangesPerPair: 40, Evaluations: 50000}
}

func mustFingerprint(t *testing.T, b model.Bundle, l model.Limits) model.Digest {
	t.Helper()
	d, err := model.ComputeFingerprint(b, l)
	if err != nil {
		t.Fatalf("ComputeFingerprint: %v", err)
	}
	if !digestForm.MatchString(string(d)) {
		t.Fatalf("fingerprint %q is not sha256: and 64 lowercase hex digits", d)
	}
	return d
}

type bundleChange struct {
	name   string
	change func(b *model.Bundle)
}

func TestComputeFingerprintIgnores(t *testing.T) {
	common := []bundleChange{
		{"nothing", func(*model.Bundle) {}},
		{"fingerprint field", func(b *model.Bundle) { b.Fingerprint = "" }},
		{"source order", func(b *model.Bundle) { slices.Reverse(b.Sources) }},
		{"snapshot order", func(b *model.Bundle) { slices.Reverse(b.Snapshots) }},
		{"problem order", func(b *model.Bundle) { slices.Reverse(b.Problems) }},
		{"used namespace order", func(b *model.Bundle) { slices.Reverse(b.UsedNamespaces["payments/api"]) }},
		{"snapshot objects", func(b *model.Bundle) { b.Snapshots[1].Objects = nil }},
		{"snapshot revision", func(b *model.Bundle) { b.Snapshots[1].Revision = "other" }},
		{"snapshot read times", func(b *model.Bundle) {
			b.Snapshots[0].ReadStartedAt, b.Snapshots[0].ReadFinishedAt = time.Now(), time.Now()
		}},
		{"snapshot problems", func(b *model.Bundle) { b.Snapshots[1].Problems = nil }},
		{"snapshot contract version", func(b *model.Bundle) { b.Snapshots[1].ContractVersion = 2 }},
		{"snapshot pull request", func(b *model.Bundle) { b.Snapshots[1].PullRequest = &model.PullRequest{Number: 7, HeadSHA: "x"} }},
		{"problem message", func(b *model.Bundle) { b.Problems[0].Message = "other" }},
		{"problem origin", func(b *model.Bundle) { b.Problems[0].Origin = nil }},
		{"forbidden rules at one version", func(b *model.Bundle) { b.Rules.Forbidden = nil }},
		{"hygiene rules at one version", func(b *model.Bundle) { b.Rules.Hygiene[0].Name = "renamed" }},
		{"tool version", func(b *model.Bundle) { b.ToolVersion = "v9.9.9" }},
	}
	bases := []struct {
		name    string
		bundle  func() model.Bundle
		changes []bundleChange
	}{
		{"pull request", fingerprintBundle, []bundleChange{
			{"head SHA", func(b *model.Bundle) { b.PullRequest.HeadSHA = "other" }},
			{"merge SHA", func(b *model.Bundle) { b.PullRequest.MergeSHA = "other" }},
			{"target branch", func(b *model.Bundle) { b.PullRequest.TargetBranch = "release" }},
			{"before content", func(b *model.Bundle) {
				b.Before = &model.Analysis{
					ID: 99, Fingerprint: sha("before"), ToolVersion: "v8", Status: model.AnalysisPartial,
					Endpoints: []model.Endpoint{{Key: "internet"}},
					Problems:  []model.Problem{{Reason: model.ReasonUnowned, Subject: "z"}},
				}
			}},
		}},
		{"baseline", baselineBundle, []bundleChange{
			{"no before", func(b *model.Bundle) { b.Before = nil }},
			{"other before", func(b *model.Bundle) { b.Before = &model.Analysis{ID: 9, Fingerprint: sha("other")} }},
			{"before without a fingerprint", func(b *model.Bundle) { b.Before = &model.Analysis{} }},
		}},
	}
	for _, base := range bases {
		t.Run(base.name, func(t *testing.T) {
			want := mustFingerprint(t, base.bundle(), fingerprintLimits())
			for _, tt := range slices.Concat(common, base.changes) {
				t.Run(tt.name, func(t *testing.T) {
					b := base.bundle()
					tt.change(&b)
					if got := mustFingerprint(t, b, fingerprintLimits()); got != want {
						t.Errorf("fingerprint = %s, want %s", got, want)
					}
				})
			}
		})
	}
}

func TestComputeFingerprintBeforeAbsent(t *testing.T) {
	without := fingerprintBundle()
	without.Before = nil
	empty := fingerprintBundle()
	empty.Before = &model.Analysis{ID: 4}
	a, b := mustFingerprint(t, without, fingerprintLimits()), mustFingerprint(t, empty, fingerprintLimits())
	if a != b {
		t.Errorf("no Before gives %s, a Before without a fingerprint %s", a, b)
	}
}

func TestComputeFingerprintNilAndEmpty(t *testing.T) {
	tests := []struct {
		name string
		set  func(b *model.Bundle, empty bool)
	}{
		{"sources", func(b *model.Bundle, empty bool) { b.Sources = emptyOrNil[model.Source](empty) }},
		{"snapshots", func(b *model.Bundle, empty bool) { b.Snapshots = emptyOrNil[model.Snapshot](empty) }},
		{"problems", func(b *model.Bundle, empty bool) { b.Problems = emptyOrNil[model.Problem](empty) }},
		{"used namespaces", func(b *model.Bundle, empty bool) {
			b.UsedNamespaces = nil
			if empty {
				b.UsedNamespaces = map[model.SourceID][]string{}
			}
		}},
		{"used namespaces of a source", func(b *model.Bundle, empty bool) {
			b.UsedNamespaces["payments/api"] = emptyOrNil[string](empty)
		}},
		{"kept tag keys of a source", func(b *model.Bundle, empty bool) { b.Sources[0].KeptTagKeys = emptyOrNil[string](empty) }},
		{"helm value files", func(b *model.Bundle, empty bool) { b.Sources[2].Git.Helm.ValueFiles = emptyOrNil[string](empty) }},
		{"helm parameters", func(b *model.Bundle, empty bool) {
			b.Sources[2].Git.Helm.Parameters = emptyOrNil[model.HelmParameter](empty)
		}},
		{"node group subnets", func(b *model.Bundle, empty bool) { b.Profile.NodeGroups[0].Subnets = emptyOrNil[string](empty) }},
		{"node group security groups", func(b *model.Bundle, empty bool) {
			b.Profile.NodeGroups[0].SecurityGroups = emptyOrNil[string](empty)
		}},
		{"private network prefixes", func(b *model.Bundle, empty bool) {
			b.Profile.PrivateNetworks[0].Prefixes = emptyOrNil[netip.Prefix](empty)
		}},
		{"link-local network prefixes", func(b *model.Bundle, empty bool) {
			b.Profile.LinkLocalNetworks[0].Prefixes = emptyOrNil[netip.Prefix](empty)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withNil, withEmpty := fingerprintBundle(), fingerprintBundle()
			tt.set(&withNil, false)
			tt.set(&withEmpty, true)
			a := mustFingerprint(t, withNil, fingerprintLimits())
			b := mustFingerprint(t, withEmpty, fingerprintLimits())
			if a != b {
				t.Errorf("nil gives %s, empty %s", a, b)
			}
		})
	}
}

func emptyOrNil[T any](empty bool) []T {
	if empty {
		return []T{}
	}
	return nil
}

func TestComputeFingerprintCovers(t *testing.T) {
	prefix := netip.MustParsePrefix
	git := func(b *model.Bundle) *model.GitSource { return b.Sources[2].Git }
	changes := []bundleChange{
		{"kind", func(b *model.Bundle) { b.Kind = model.AnalysisBaseline }},
		{"environment", func(b *model.Bundle) { b.Environment = "staging" }},
		{"pull request repository", func(b *model.Bundle) { b.PullRequest.RepoURL = "https://github.com/acme/other" }},
		{"pull request number", func(b *model.Bundle) { b.PullRequest.Number = 8 }},
		{"source removed", func(b *model.Bundle) { b.Sources = b.Sources[1:] }},
		{"source ID", func(b *model.Bundle) { b.Sources[2].ID = "payments/web" }},
		{"source kind", func(b *model.Bundle) { b.Sources[1].Kind = model.SourceGit }},
		{"source environment", func(b *model.Bundle) { b.Sources[2].Environment = "staging" }},
		{"source owner team", func(b *model.Bundle) { b.Sources[2].Owner.Team = "billing" }},
		{"source owner application", func(b *model.Bundle) { b.Sources[2].Owner.Application = "web" }},
		{"git repository", func(b *model.Bundle) { git(b).RepoURL = "https://github.com/acme/other" }},
		{"git path", func(b *model.Bundle) { git(b).Path = "other" }},
		{"git revision", func(b *model.Bundle) { git(b).TargetRevision = "release" }},
		{"git render", func(b *model.Bundle) { git(b).Render = model.RenderPlain }},
		{"git namespace", func(b *model.Bundle) { git(b).Namespace = "other" }},
		{"helm removed", func(b *model.Bundle) { git(b).Helm = nil }},
		{"helm release", func(b *model.Bundle) { git(b).Helm.ReleaseName = "web" }},
		{"helm value file", func(b *model.Bundle) { git(b).Helm.ValueFiles[0] = "other.yaml" }},
		// Value files and parameters apply in order.
		{"helm value file order", func(b *model.Bundle) { slices.Reverse(git(b).Helm.ValueFiles) }},
		{"helm parameter order", func(b *model.Bundle) { slices.Reverse(git(b).Helm.Parameters) }},
		{"helm parameter name", func(b *model.Bundle) { git(b).Helm.Parameters[0].Name = "c" }},
		{"helm parameter value", func(b *model.Bundle) { git(b).Helm.Parameters[0].Value = "3" }},
		{"helm parameter force string", func(b *model.Bundle) { git(b).Helm.Parameters[0].ForceString = true }},
		{"aws region", func(b *model.Bundle) { b.Sources[0].AWS.Region = "us-east-1" }},
		{"aws VPC", func(b *model.Bundle) { b.Sources[0].AWS.VPCID = "vpc-0def" }},
		{"aws cluster name", func(b *model.Bundle) { b.Sources[0].AWS.ClusterName = "other" }},
		{"cluster source", func(b *model.Bundle) { b.Sources[1].Cluster = nil }},
		{"source kept tag keys", func(b *model.Bundle) { b.Sources[0].KeptTagKeys = []string{"app"} }},
		{"snapshot removed", func(b *model.Bundle) { b.Snapshots = b.Snapshots[1:] }},
		{"snapshot key source", func(b *model.Bundle) { b.Snapshots[0].Key.Source = "aws/staging" }},
		{"snapshot key pull request", func(b *model.Bundle) { b.Snapshots[1].Key.PullRequest = 0 }},
		{"snapshot digest", func(b *model.Bundle) { b.Snapshots[1].Digest = sha("other") }},
		{"used namespace", func(b *model.Bundle) { b.UsedNamespaces["payments/api"] = []string{"payments"} }},
		{"used namespaces source", func(b *model.Bundle) {
			b.UsedNamespaces = map[model.SourceID][]string{"payments/web": {"jobs", "payments"}}
		}},
		{"problem reason", func(b *model.Bundle) { b.Problems[2].Reason = model.ReasonDigestMismatch }},
		{"problem subject", func(b *model.Bundle) { b.Problems[2].Subject = "other" }},
		{"problem removed", func(b *model.Bundle) { b.Problems = b.Problems[1:] }},
		{"profile CNI", func(b *model.Bundle) { b.Profile.CNI = "calico" }},
		{"profile enforcement", func(b *model.Bundle) { b.Profile.NetworkPolicyEnforcement = model.ToggleOff }},
		{"profile mode", func(b *model.Bundle) { b.Profile.NetworkPolicyMode = "strict" }},
		{"profile pod security groups", func(b *model.Bundle) { b.Profile.PodSecurityGroups = model.ToggleOn }},
		{"profile external SNAT", func(b *model.Bundle) { b.Profile.ExternalSNAT = model.ToggleUnknown }},
		{"profile excluded SNAT", func(b *model.Bundle) { b.Profile.ExcludeSNATCIDRs = []netip.Prefix{prefix("10.2.0.0/16")} }},
		{"profile custom networking", func(b *model.Bundle) { b.Profile.CustomNetworking = model.ToggleOff }},
		{"profile ephemeral ports", func(b *model.Bundle) { b.Profile.EphemeralPorts.From = 32768 }},
		{"profile node group subnet", func(b *model.Bundle) { b.Profile.NodeGroups[0].Subnets[0] = "subnet-b" }},
		{"profile node group security group", func(b *model.Bundle) { b.Profile.NodeGroups[0].SecurityGroups[0] = "sg-0def" }},
		{"profile node group labels", func(b *model.Bundle) { b.Profile.NodeGroups[0].Labels = map[string]string{"a": "b"} }},
		{"profile node group taints", func(b *model.Bundle) {
			b.Profile.NodeGroups[0].Taints = []model.Taint{{Key: "k", Effect: "NoSchedule"}}
		}},
		{"profile pod CIDRs", func(b *model.Bundle) { b.Profile.PodCIDRs = []netip.Prefix{prefix("10.245.0.0/16")} }},
		{"profile ipBlock", func(b *model.Bundle) { b.Profile.IPBlockAppliesToPods = model.ToggleOn }},
		{"profile not internet", func(b *model.Bundle) { b.Profile.NotInternet = []netip.Prefix{prefix("198.51.100.0/24")} }},
		{"profile private network", func(b *model.Bundle) { b.Profile.PrivateNetworks[0].Name = "office" }},
		{"profile private network prefix", func(b *model.Bundle) { b.Profile.PrivateNetworks[0].Prefixes[0] = prefix("10.8.0.0/16") }},
		{"profile link-local network", func(b *model.Bundle) { b.Profile.LinkLocalNetworks = nil }},
		{"profile namespace labels", func(b *model.Bundle) { b.Profile.NamespaceLabels["payments"]["team"] = "billing" }},
		{"profile kept tag keys", func(b *model.Bundle) { b.Profile.KeptTagKeys = []string{"app"} }},
		{"profile value sources", func(b *model.Bundle) { b.Profile.Sources = map[string]model.ProfileSource{"cni": model.ProfileFromAWS} }},
		{"rules version", func(b *model.Bundle) { b.Rules.Version = 4 }},
		{"schema version", func(b *model.Bundle) { b.SchemaVersion++ }},
		{"algorithm version", func(b *model.Bundle) { b.AlgorithmVersion++ }},
		{"before fingerprint", func(b *model.Bundle) { b.Before.Fingerprint = sha("other") }},
		{"before removed", func(b *model.Bundle) { b.Before = nil }},
	}
	want := mustFingerprint(t, fingerprintBundle(), fingerprintLimits())
	for _, tt := range changes {
		t.Run(tt.name, func(t *testing.T) {
			b := fingerprintBundle()
			tt.change(&b)
			if got := mustFingerprint(t, b, fingerprintLimits()); got == want {
				t.Errorf("fingerprint unchanged: %s", got)
			}
		})
	}
}

func TestComputeFingerprintCoversLimits(t *testing.T) {
	tests := []struct {
		name   string
		change func(l *model.Limits)
	}{
		{"endpoints", func(l *model.Limits) { l.Endpoints++ }},
		{"rules", func(l *model.Limits) { l.Rules++ }},
		{"prefixes per endpoint", func(l *model.Limits) { l.PrefixesPerEndpoint++ }},
		{"port ranges per pair", func(l *model.Limits) { l.PortRangesPerPair++ }},
		{"evaluations", func(l *model.Limits) { l.Evaluations++ }},
	}
	want := mustFingerprint(t, fingerprintBundle(), fingerprintLimits())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := fingerprintLimits()
			tt.change(&l)
			if got := mustFingerprint(t, fingerprintBundle(), l); got == want {
				t.Errorf("fingerprint unchanged: %s", got)
			}
		})
	}
}

func TestComputeFingerprintNumbers(t *testing.T) {
	type change = func(b *model.Bundle, l *model.Limits, n int)
	fields := []struct {
		name string
		set  change
	}{
		{"rules version", func(b *model.Bundle, _ *model.Limits, n int) { b.Rules.Version = int64(n) }},
		{"pull request number", func(b *model.Bundle, _ *model.Limits, n int) { b.PullRequest.Number = n }},
		{"schema version", func(b *model.Bundle, _ *model.Limits, n int) { b.SchemaVersion = n }},
		{"algorithm version", func(b *model.Bundle, _ *model.Limits, n int) { b.AlgorithmVersion = n }},
		{"endpoints limit", func(_ *model.Bundle, l *model.Limits, n int) { l.Endpoints = n }},
		{"rules limit", func(_ *model.Bundle, l *model.Limits, n int) { l.Rules = n }},
		{"prefixes limit", func(_ *model.Bundle, l *model.Limits, n int) { l.PrefixesPerEndpoint = n }},
		{"port ranges limit", func(_ *model.Bundle, l *model.Limits, n int) { l.PortRangesPerPair = n }},
		{"evaluations limit", func(_ *model.Bundle, l *model.Limits, n int) { l.Evaluations = n }},
	}
	for _, f := range fields {
		t.Run(f.name, func(t *testing.T) {
			for _, n := range []int{aboveExact, -aboveExact} {
				b, l := fingerprintBundle(), fingerprintLimits()
				f.set(&b, &l, n)
				if got, err := model.ComputeFingerprint(b, l); !errors.Is(err, model.ErrNumberTooLarge) {
					t.Errorf("%d: ComputeFingerprint = %s, %v; want ErrNumberTooLarge", n, got, err)
				}
			}
			b, l := fingerprintBundle(), fingerprintLimits()
			f.set(&b, &l, 1<<53)
			mustFingerprint(t, b, l)
		})
	}
}

func TestComputeFingerprintLeavesOperands(t *testing.T) {
	unsorted := func() model.Bundle {
		b := fingerprintBundle()
		slices.Reverse(b.Sources)
		slices.Reverse(b.Snapshots)
		slices.Reverse(b.Problems)
		slices.Reverse(b.UsedNamespaces["payments/api"])
		return b
	}
	b, l := unsorted(), fingerprintLimits()
	first := mustFingerprint(t, b, l)
	if want := unsorted(); !reflect.DeepEqual(b, want) {
		t.Errorf("ComputeFingerprint changed its Bundle:\ngot  %+v\nwant %+v", b, want)
	}
	if l != fingerprintLimits() {
		t.Errorf("ComputeFingerprint changed its limits to %+v", l)
	}
	for range 10 {
		if got := mustFingerprint(t, b, l); got != first {
			t.Fatalf("a later call gave %s, the first %s", got, first)
		}
	}
}
