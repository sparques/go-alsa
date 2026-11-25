package config

import (
	"fmt"
	"strconv"
	"strings"
)

const maximumNesting = 64

type parser struct {
	tokens []token
	index  int
	source string
}

func (p *parser) statements(end tokenKind, depth int) ([]Statement, error) {
	if depth > maximumNesting {
		return nil, p.error(p.peek(), "configuration nesting exceeds 64")
	}
	var result []Statement
	for {
		tok := p.peek()
		if tok.Kind == end {
			if end != tokenEOF {
				p.index++
			}
			return result, nil
		}
		if tok.Kind == tokenEOF {
			return nil, p.error(tok, "unexpected end of configuration")
		}
		if tok.Kind == tokenComma || tok.Kind == tokenSemicolon {
			p.index++
			continue
		}
		if tok.Kind == tokenInclude {
			p.index++
			result = append(result, Statement{Position: p.publicPosition(tok.Position), Include: tok.Text})
			continue
		}
		if tok.Kind != tokenWord && tok.Kind != tokenString {
			return nil, p.error(tok, "expected configuration key")
		}
		p.index++
		name, path, mode := assignmentName(tok.Text, tok.Kind == tokenString)
		if name == "" {
			return nil, p.error(tok, "empty configuration key")
		}
		value, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		assignment := &Assignment{Name: name, RawName: tok.Text, Path: path, Mode: mode, Position: p.publicPosition(tok.Position), Value: value}
		result = append(result, Statement{Position: assignment.Position, Assignment: assignment})
	}
}

func (p *parser) value(depth int) (Value, error) {
	if depth > maximumNesting {
		return Value{}, p.error(p.peek(), "configuration nesting exceeds 64")
	}
	tok := p.peek()
	p.index++
	position := p.publicPosition(tok.Position)
	switch tok.Kind {
	case tokenString:
		return Value{Kind: ValueString, Position: position, String: tok.Text}, nil
	case tokenWord:
		return scalarValue(tok.Text, position), nil
	case tokenLeftBrace:
		entries, err := p.statements(tokenRightBrace, depth)
		return Value{Kind: ValueCompound, Position: position, Entries: entries}, err
	case tokenLeftBracket:
		items, err := p.array(depth)
		return Value{Kind: ValueArray, Position: position, Items: items}, err
	case tokenEOF:
		return Value{}, p.error(tok, "missing configuration value")
	default:
		return Value{}, p.error(tok, "expected configuration value")
	}
}

func (p *parser) array(depth int) ([]Value, error) {
	var result []Value
	for {
		tok := p.peek()
		if tok.Kind == tokenRightBracket {
			p.index++
			return result, nil
		}
		if tok.Kind == tokenEOF {
			return nil, p.error(tok, "unexpected end of array")
		}
		if tok.Kind == tokenComma || tok.Kind == tokenSemicolon {
			p.index++
			continue
		}
		if tok.Kind == tokenInclude {
			return nil, p.error(tok, "include is not an array value")
		}
		value, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
}

func scalarValue(text string, position Position) Value {
	switch strings.ToLower(text) {
	case "true", "yes", "on":
		return Value{Kind: ValueBoolean, Position: position, Boolean: true}
	case "false", "no", "off":
		return Value{Kind: ValueBoolean, Position: position, Boolean: false}
	}
	if integer, err := strconv.ParseInt(text, 0, 64); err == nil {
		return Value{Kind: ValueInteger, Position: position, Integer: integer}
	}
	if strings.ContainsAny(text, ".eE") {
		if real, err := strconv.ParseFloat(text, 64); err == nil {
			return Value{Kind: ValueReal, Position: position, Real: real}
		}
	}
	return Value{Kind: ValueString, Position: position, String: text}
}

func assignmentName(raw string, literal bool) (string, []string, AssignmentMode) {
	mode := AssignNormal
	parts := []string{raw}
	if !literal {
		parts = strings.Split(raw, ".")
	}
	for index, part := range parts {
		if part == "" {
			continue
		}
		switch part[0] {
		case '!':
			mode, parts[index] = AssignOverride, part[1:]
		case '?':
			mode, parts[index] = AssignDefault, part[1:]
		case '+':
			mode, parts[index] = AssignMerge, part[1:]
		case '-':
			mode, parts[index] = AssignRemove, part[1:]
		default:
			continue
		}
		break
	}
	return strings.Join(parts, "."), parts, mode
}

func (p *parser) peek() token { return p.tokens[p.index] }

func (p *parser) error(tok token, message string) error {
	return &syntaxError{Position: tok.Position, Message: fmt.Sprintf("%s near %q", message, tok.Text)}
}

func (p *parser) publicPosition(value position) Position {
	return Position{Source: p.source, Offset: value.Offset, Line: value.Line, Column: value.Column}
}
