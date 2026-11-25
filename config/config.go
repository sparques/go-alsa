package config

// Position identifies a byte and human-readable line/column in one source.
type Position struct {
	Offset int
	Line   int
	Column int
}

// AssignmentMode preserves libasound's prefix policy for later evaluation.
type AssignmentMode uint8

const (
	AssignNormal   AssignmentMode = iota
	AssignOverride                // !name
	AssignDefault                 // ?name
	AssignMerge                   // +name
	AssignRemove                  // -name
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
	Mode     AssignmentMode
	Position Position
	Value    Value
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
	p := parser{tokens: tokens}
	statements, err := p.statements(tokenEOF, 0)
	if err != nil {
		return nil, err
	}
	return &Document{Source: source, Statements: statements}, nil
}
