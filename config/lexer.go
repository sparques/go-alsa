package config

import "fmt"

const maximumConfigBytes = 16 << 20
const maximumTokens = 1 << 20

type tokenKind uint8

const (
	tokenEOF tokenKind = iota
	tokenWord
	tokenString
	tokenInclude
	tokenLeftBrace
	tokenRightBrace
	tokenLeftBracket
	tokenRightBracket
	tokenComma
	tokenSemicolon
	tokenEqual
)

type position struct {
	Offset int
	Line   int
	Column int
}

type token struct {
	Kind     tokenKind
	Text     string
	Position position
}

type syntaxError struct {
	Position position
	Message  string
}

func (e *syntaxError) Error() string {
	return fmt.Sprintf("alsa config: %d:%d: %s", e.Position.Line, e.Position.Column, e.Message)
}

type lexer struct {
	input  string
	offset int
	line   int
	column int
}

func lex(input string) ([]token, error) {
	if len(input) > maximumConfigBytes {
		return nil, &syntaxError{Position: position{Line: 1, Column: 1}, Message: "configuration exceeds 16 MiB"}
	}
	l := lexer{input: input, line: 1, column: 1}
	var tokens []token
	for {
		if len(tokens) == maximumTokens {
			return nil, &syntaxError{Position: l.position(), Message: "configuration exceeds token limit"}
		}
		tok, err := l.next()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
		if tok.Kind == tokenEOF {
			return tokens, nil
		}
	}
}

func (l *lexer) next() (token, error) {
	l.skipSpaceAndComments()
	start := l.position()
	if l.offset == len(l.input) {
		return token{Kind: tokenEOF, Position: start}, nil
	}
	character := l.input[l.offset]
	switch character {
	case '{':
		l.advance()
		return token{Kind: tokenLeftBrace, Text: "{", Position: start}, nil
	case '}':
		l.advance()
		return token{Kind: tokenRightBrace, Text: "}", Position: start}, nil
	case '[':
		l.advance()
		return token{Kind: tokenLeftBracket, Text: "[", Position: start}, nil
	case ']':
		l.advance()
		return token{Kind: tokenRightBracket, Text: "]", Position: start}, nil
	case ',':
		l.advance()
		return token{Kind: tokenComma, Text: ",", Position: start}, nil
	case ';':
		l.advance()
		return token{Kind: tokenSemicolon, Text: ";", Position: start}, nil
	case '=':
		l.advance()
		return token{Kind: tokenEqual, Text: "=", Position: start}, nil
	case '\'', '"':
		text, err := l.quoted(character, start)
		return token{Kind: tokenString, Text: text, Position: start}, err
	case '<':
		text, err := l.include(start)
		return token{Kind: tokenInclude, Text: text, Position: start}, err
	default:
		begin := l.offset
		for l.offset < len(l.input) && !isDelimiter(l.input[l.offset]) {
			l.advance()
		}
		if begin == l.offset {
			return token{}, &syntaxError{Position: start, Message: fmt.Sprintf("unexpected byte %#x", character)}
		}
		return token{Kind: tokenWord, Text: l.input[begin:l.offset], Position: start}, nil
	}
}

func (l *lexer) quoted(quote byte, start position) (string, error) {
	l.advance()
	result := make([]byte, 0, 32)
	for l.offset < len(l.input) {
		character := l.input[l.offset]
		if character == quote {
			l.advance()
			return string(result), nil
		}
		if character == '\n' || character == '\r' {
			return "", &syntaxError{Position: start, Message: "unterminated quoted string"}
		}
		l.advance()
		if character != '\\' {
			result = append(result, character)
			continue
		}
		if l.offset == len(l.input) {
			return "", &syntaxError{Position: start, Message: "unterminated escape"}
		}
		escaped := l.input[l.offset]
		l.advance()
		if escaped == '\n' {
			continue
		}
		if escaped == '\r' {
			if l.offset < len(l.input) && l.input[l.offset] == '\n' {
				l.advance()
			}
			continue
		}
		switch escaped {
		case 'n':
			result = append(result, '\n')
		case 'r':
			result = append(result, '\r')
		case 't':
			result = append(result, '\t')
		default:
			result = append(result, escaped)
		}
	}
	return "", &syntaxError{Position: start, Message: "unterminated quoted string"}
}

func (l *lexer) include(start position) (string, error) {
	l.advance()
	begin := l.offset
	for l.offset < len(l.input) && l.input[l.offset] != '>' {
		if l.input[l.offset] == '\n' || l.input[l.offset] == '\r' {
			return "", &syntaxError{Position: start, Message: "unterminated include"}
		}
		l.advance()
	}
	if l.offset == len(l.input) {
		return "", &syntaxError{Position: start, Message: "unterminated include"}
	}
	text := l.input[begin:l.offset]
	l.advance()
	if text == "" {
		return "", &syntaxError{Position: start, Message: "empty include"}
	}
	return text, nil
}

func (l *lexer) skipSpaceAndComments() {
	for {
		for l.offset < len(l.input) && isSpace(l.input[l.offset]) {
			l.advance()
		}
		if l.offset == len(l.input) {
			return
		}
		if l.input[l.offset] == '#' {
			l.skipLine()
			continue
		}
		if l.offset+1 < len(l.input) && l.input[l.offset:l.offset+2] == "//" {
			l.skipLine()
			continue
		}
		return
	}
}

func (l *lexer) skipLine() {
	for l.offset < len(l.input) && l.input[l.offset] != '\n' {
		l.advance()
	}
}

func (l *lexer) advance() {
	if l.input[l.offset] == '\n' {
		l.line++
		l.column = 1
	} else {
		l.column++
	}
	l.offset++
}

func (l *lexer) position() position {
	return position{Offset: l.offset, Line: l.line, Column: l.column}
}

func isSpace(character byte) bool {
	return character == ' ' || character == '\t' || character == '\v' || character == '\f' || character == '\r' || character == '\n'
}

func isDelimiter(character byte) bool {
	return isSpace(character) || character == '{' || character == '}' || character == '[' || character == ']' || character == ',' || character == ';' || character == '=' || character == '\'' || character == '"' || character == '<' || character == '#'
}
