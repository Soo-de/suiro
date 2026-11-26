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
