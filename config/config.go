package config

import (
	"errors"
	"strings"
)

// ErrNotFound reports a requested configuration assignment that does not
// exist. Callers can use errors.Is while retaining the missing path in the
// returned error.
var ErrNotFound = errors.New("alsa config: assignment not found")

// ErrGraphCycle reports a named PCM graph whose slave chain refers back to an
// active node.
var ErrGraphCycle = errors.New("alsa config: PCM graph cycle")

// Position identifies a byte and human-readable line/column in one source.
type Position struct {
	Source string
	Offset int
	Line   int
	Column int
}

// AssignmentMode preserves libasound's prefix policy for later evaluation.
type AssignmentMode uint8

const (
	AssignMergeCreate AssignmentMode = iota // default and +name
	AssignMerge                             // -name; existing node required
	AssignNoOverride                        // ?name
	AssignOverride                          // !name

	AssignNormal  = AssignMergeCreate
	AssignDefault = AssignNoOverride
)

// ValueKind identifies a parsed value without applying configuration policy.
type ValueKind uint8

const (
	ValueString ValueKind = iota
	ValueInteger
	ValueReal
	ValueBoolean
	ValueCompound
	ValueArray
)

// Value is one typed scalar, compound, or array value.
type Value struct {
	Kind     ValueKind
	Position Position
	String   string
	Integer  int64
	Real     float64
	Boolean  bool
	Entries  []Statement
	Items    []Value
}

// Assignment binds a possibly dotted name to a value. Name is canonicalized
// by removing its assignment modifier; RawName preserves the source spelling.
type Assignment struct {
	Name     string
	RawName  string
	Path     []string
	Mode     AssignmentMode
	Position Position
	Value    Value
}

// Assignments returns every assignment to a canonical dotted path in source
// order. Nested compound keys are indexed beneath their parent path.
func (d *Document) Assignments(name string) []*Assignment {
	if d == nil {
		return nil
	}
	var result []*Assignment
	collectAssignments(d.Statements, nil, name, &result)
	return result
}

// Lookup returns the last assignment to name. Assignment-mode evaluation is
// intentionally separate; Assignments exposes all definitions when policy is
// significant.
func (d *Document) Lookup(name string) (*Assignment, bool) {
	items := d.Assignments(name)
	if len(items) == 0 {
		return nil, false
	}
	return items[len(items)-1], true
}

func collectAssignments(statements []Statement, prefix []string, wanted string, result *[]*Assignment) {
	for _, statement := range statements {
		assignment := statement.Assignment
		if assignment == nil {
			continue
		}
		path := append(append([]string(nil), prefix...), assignment.Path...)
		if strings.Join(path, ".") == wanted {
			*result = append(*result, assignment)
		}
		if assignment.Value.Kind == ValueCompound {
			collectAssignments(assignment.Value.Entries, path, wanted, result)
		}
	}
}

// Statement is either an assignment or an include, preserving source order.
type Statement struct {
	Position   Position
	Assignment *Assignment
	Include    string
}

// Document is a parsed but unevaluated ALSA configuration source.
type Document struct {
	Source     string
	Statements []Statement
}

// Parse parses one configuration source without reading includes or executing
// functions. Source is retained for diagnostics.
func Parse(source, input string) (*Document, error) {
	tokens, err := lex(input)
	if err != nil {
		return nil, err
	}
	p := parser{tokens: tokens, source: source}
	statements, err := p.statements(tokenEOF, 0)
	if err != nil {
		return nil, err
	}
	return &Document{Source: source, Statements: statements}, nil
}
