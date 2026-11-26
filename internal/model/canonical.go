package model

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/gowebpki/jcs"
)

// canonicalDigest returns the SHA-256 of the RFC 8785 form of v's JSON. It
// errors on a number beyond ±2^53, which canonicalizing would round silently.
func canonicalDigest(v any) (Digest, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("model: encode for hashing: %w", err)
	}
	if err := checkNumbers(data); err != nil {
		return "", err
	}
	canonical, err := jcs.Transform(data)
	if err != nil {
		return "", fmt.Errorf("model: canonicalize: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return Digest("sha256:" + hex.EncodeToString(sum[:])), nil
}

// checkNumbers errors with ErrNumberTooLarge when the JSON in data holds a
// number beyond ±2^53.
func checkNumbers(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	for {
		token, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("model: read JSON: %w", err)
		}
		if n, ok := token.(json.Number); ok && beyondSafeRange(string(n)) {
			return ErrNumberTooLarge
		}
	}
}

// maxSafeInteger is 2^53 written out.
const maxSafeInteger = "9007199254740992"

// beyondSafeRange reports whether the JSON number literal lies outside ±2^53.
func beyondSafeRange(literal string) bool {
	// An out-of-range literal parses to ±Inf or 0, which compare correctly.
	f, _ := strconv.ParseFloat(literal, 64)
	if abs := math.Abs(f); abs != 1<<53 {
		return abs > 1<<53
	}
	// The float rounds to exactly 2^53, so only the digits can tell whether
	// the literal is above it.
	mantissa, exponent := strings.TrimPrefix(literal, "-"), 0
	if i := strings.IndexAny(mantissa, "eE"); i >= 0 {
		e, err := strconv.Atoi(mantissa[i+1:])
		if err != nil {
			return true
		}
		mantissa, exponent = mantissa[:i], e
	}
	whole, fraction, _ := strings.Cut(mantissa, ".")
	digits := strings.TrimLeft(whole+fraction, "0")
	significant := strings.TrimRight(digits, "0")
	// width is the number of digits before the decimal point.
	width := len(digits) + exponent - len(fraction)
	return width > len(maxSafeInteger) || width == len(maxSafeInteger) && significant > maxSafeInteger
}

// fingerprintInput is what ComputeFingerprint hashes: Bundle with the
// reductions its doc names, plus the limits.
type fingerprintInput struct {
	Kind             AnalysisKind            `json:"kind"`
	Environment      string                  `json:"environment"`
	PullRequest      *fingerprintPullRequest `json:"pullRequest,omitempty"`
	Sources          []Source                `json:"sources"`
	Snapshots        []fingerprintSnapshot   `json:"snapshots"`
	UsedNamespaces   map[SourceID][]string   `json:"usedNamespaces"`
	Problems         []fingerprintProblem    `json:"problems"`
	Profile          ClusterProfile          `json:"profile"`
	Rules            fingerprintRules        `json:"rules"`
	Before           *fingerprintBefore      `json:"before,omitempty"`
	SchemaVersion    int                     `json:"schemaVersion"`
	AlgorithmVersion int                     `json:"algorithmVersion"`
	Limits           Limits                  `json:"limits"`
}

type fingerprintPullRequest struct {
	RepoURL string `json:"repoURL"`
	Number  int    `json:"number"`
}

type fingerprintSnapshot struct {
	Key    ReadKey `json:"key"`
	Digest Digest  `json:"digest"`
}

type fingerprintProblem struct {
	Reason  ReasonCode `json:"reason"`
	Subject string     `json:"subject"`
}

type fingerprintRules struct {
	Version int64 `json:"version"`
}

type fingerprintBefore struct {
	Fingerprint Digest `json:"fingerprint"`
}

// newFingerprintInput reduces b into fresh lists, leaving b as it is.
func newFingerprintInput(b Bundle, l Limits) fingerprintInput {
	in := fingerprintInput{
		Kind:             b.Kind,
		Environment:      b.Environment,
		Sources:          sortedSources(b.Sources),
		Snapshots:        reducedSnapshots(b.Snapshots),
		UsedNamespaces:   normalizedNamespaces(b.UsedNamespaces),
		Problems:         reducedProblems(b.Problems),
		Profile:          normalizedProfile(b.Profile),
		Rules:            fingerprintRules{Version: b.Rules.Version},
		SchemaVersion:    b.SchemaVersion,
		AlgorithmVersion: b.AlgorithmVersion,
		Limits:           l,
	}
	if pr := b.PullRequest; pr != nil {
		in.PullRequest = &fingerprintPullRequest{RepoURL: pr.RepoURL, Number: pr.Number}
	}
	if b.Kind == AnalysisPullRequest {
		in.Before = &fingerprintBefore{}
		if b.Before != nil {
			in.Before.Fingerprint = b.Before.Fingerprint
		}
	}
	return in
}

func sortedSources(sources []Source) []Source {
	out := slices.Clone(sources)
	slices.SortStableFunc(out, func(a, b Source) int { return cmp.Compare(a.ID, b.ID) })
	return nonNil(out)
}

func reducedSnapshots(snapshots []Snapshot) []fingerprintSnapshot {
	out := make([]fingerprintSnapshot, len(snapshots))
	for i, s := range snapshots {
		out[i] = fingerprintSnapshot{Key: s.Key, Digest: s.Digest}
	}
	slices.SortStableFunc(out, func(a, b fingerprintSnapshot) int {
		return cmp.Or(cmp.Compare(a.Key.Source, b.Key.Source), cmp.Compare(a.Key.PullRequest, b.Key.PullRequest))
	})
	return out
}

func reducedProblems(problems []Problem) []fingerprintProblem {
	out := make([]fingerprintProblem, len(problems))
	for i, p := range problems {
		out[i] = fingerprintProblem{Reason: p.Reason, Subject: p.Subject}
	}
	slices.SortFunc(out, func(a, b fingerprintProblem) int {
		return cmp.Or(cmp.Compare(a.Reason, b.Reason), cmp.Compare(a.Subject, b.Subject))
	})
	return out
}

func normalizedNamespaces(used map[SourceID][]string) map[SourceID][]string {
	out := make(map[SourceID][]string, len(used))
	for source, namespaces := range used {
		sorted := slices.Clone(namespaces)
		slices.Sort(sorted)
		out[source] = nonNil(slices.Compact(sorted))
	}
	return out
}

// normalizedProfile returns p with every list and map that its JSON always
// writes made non-nil. A new field of that sort joins this list.
func normalizedProfile(p ClusterProfile) ClusterProfile {
	p.NodeGroups = slices.Clone(p.NodeGroups)
	for i := range p.NodeGroups {
		g := &p.NodeGroups[i]
		g.Subnets, g.SecurityGroups = nonNil(g.Subnets), nonNil(g.SecurityGroups)
	}
	p.PrivateNetworks = normalizedNetworks(p.PrivateNetworks)
	p.LinkLocalNetworks = normalizedNetworks(p.LinkLocalNetworks)
	labels := make(map[string]map[string]string, len(p.NamespaceLabels))
	for namespace, l := range p.NamespaceLabels {
		if l == nil {
			l = map[string]string{}
		}
		labels[namespace] = l
	}
	p.NamespaceLabels = labels
	return p
}

func normalizedNetworks(networks []NamedNetwork) []NamedNetwork {
	out := slices.Clone(networks)
	for i := range out {
		out[i].Prefixes = nonNil(out[i].Prefixes)
	}
	return out
}

// compareOrigin orders origins as the package comment does, none first.
func compareOrigin(a, b *Origin) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	return cmp.Or(
		cmp.Compare(a.Source, b.Source),
		cmp.Compare(a.File, b.File),
		cmp.Compare(a.Line, b.Line),
		cmp.Compare(a.Resource, b.Resource),
		cmp.Compare(a.Locator, b.Locator),
	)
}

// compareProblem is the canonical order of problems.
func compareProblem(a, b Problem) int {
	return cmp.Or(
		cmp.Compare(a.Reason, b.Reason),
		cmp.Compare(a.Subject, b.Subject),
		compareOrigin(a.Origin, b.Origin),
		cmp.Compare(a.Message, b.Message),
	)
}

// nonNil returns s, or an empty slice for nil, so that it encodes as [].
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
