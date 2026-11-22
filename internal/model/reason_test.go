package model_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Soo-de/suiro/internal/model"
)

// declared returns the string constants of type typ that the package's
// non-test files declare, value to name, read from the source so the
// expectation does not come from the table under test. A value declared
// twice fails t.
func declared(t *testing.T, typ string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]string)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			// Only specs that name the type and give a string literal;
			// every constant of these types is written that way.
			for _, s := range g.Specs {
				vs := s.(*ast.ValueSpec)
				id, ok := vs.Type.(*ast.Ident)
				if !ok || id.Name != typ {
					continue
				}
				for i, name := range vs.Names {
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						t.Fatalf("%s: %s is not a string literal", fset.Position(vs.Pos()), name.Name)
					}
					v, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatal(err)
					}
					if prev, dup := out[v]; dup {
						t.Fatalf("%s and %s share the value %q", prev, name.Name, v)
					}
					out[v] = name.Name
				}
			}
		}
	}
	if len(out) == 0 {
		t.Fatalf("no constants of type %s found", typ)
	}
	return out
}

func TestReasonsCoverEveryDeclaredCode(t *testing.T) {
	codes := declared(t, "ReasonCode")
	table := model.Reasons()
	for v, name := range codes {
		if _, ok := table[model.ReasonCode(v)]; !ok {
			t.Errorf("%s (%q) has no row in Reasons", name, v)
		}
	}
	for c := range table {
		if _, ok := codes[string(c)]; !ok {
			t.Errorf("Reasons has a row for %q, which no constant declares", c)
		}
	}
}

func TestReasonRowsUseDeclaredValues(t *testing.T) {
	levels := declared(t, "ReasonLevel")
	scopes := declared(t, "ReasonScope")
	producers := declared(t, "Producer")
	for c, spec := range model.Reasons() {
		if _, ok := levels[string(spec.Level)]; !ok {
			t.Errorf("%s: level %q is not declared", c, spec.Level)
		}
		if _, ok := scopes[string(spec.Scope)]; !ok {
			t.Errorf("%s: scope %q is not declared", c, spec.Scope)
		}
		if len(spec.RaisedBy) == 0 {
			t.Errorf("%s: no producer", c)
		}
		seen := make(map[model.Producer]bool)
		for _, p := range spec.RaisedBy {
			if _, ok := producers[string(p)]; !ok {
				t.Errorf("%s: producer %q is not declared", c, p)
			}
			if seen[p] {
				t.Errorf("%s: producer %q listed twice", c, p)
			}
			seen[p] = true
		}
	}
}

func TestLevelOf(t *testing.T) {
	table := model.Reasons()
	levels := model.ReasonLevels()
	if len(levels) != len(table) {
		t.Fatalf("ReasonLevels has %d codes, Reasons %d", len(levels), len(table))
	}
	tests := []struct {
		name string
		code model.ReasonCode
		want model.ReasonLevel
	}{
		// An unknown code from a newer collector is never read as harmless.
		{"unknown code", "no-such-code-in-this-build", model.LevelInput},
		{"empty code", "", model.LevelInput},
	}
	for c, spec := range table {
		tests = append(tests, struct {
			name string
			code model.ReasonCode
			want model.ReasonLevel
		}{string(c), c, spec.Level})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := model.LevelOf(tt.code); got != tt.want {
				t.Errorf("LevelOf(%q) = %q, want %q", tt.code, got, tt.want)
			}
			if l, ok := levels[tt.code]; ok && l != tt.want {
				t.Errorf("ReasonLevels()[%q] = %q, want %q", tt.code, l, tt.want)
			}
		})
	}
}

func TestReasonsReturnsACopy(t *testing.T) {
	first := model.Reasons()
	for c, spec := range first {
		if len(spec.RaisedBy) == 0 {
			continue
		}
		want := slices.Clone(spec.RaisedBy)
		spec.RaisedBy[0] = "changed"
		delete(first, c)
		got, ok := model.Reasons()[c]
		if !ok || !slices.Equal(got.RaisedBy, want) {
			t.Fatalf("changing the result of Reasons changed the table for %s", c)
		}
		return
	}
	t.Fatal("no row with a producer")
}
