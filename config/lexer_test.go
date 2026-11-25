package config

import (
	"errors"
	"os"
	"testing"
)

func TestLexRepresentativeConfig(t *testing.T) {
	input := "# comment\n<confdir:pcm/front.conf>\npcm.!default {\n type hw\n card \"Device\\tOne\" // comment\n @args [ CARD, DEV ];\n}\n"
	tokens, err := lex(input)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		kind tokenKind
		text string
	}{
		{tokenInclude, "confdir:pcm/front.conf"}, {tokenWord, "pcm.!default"}, {tokenLeftBrace, "{"},
		{tokenWord, "type"}, {tokenWord, "hw"}, {tokenWord, "card"}, {tokenString, "Device\tOne"},
		{tokenWord, "@args"}, {tokenLeftBracket, "["}, {tokenWord, "CARD"}, {tokenComma, ","},
		{tokenWord, "DEV"}, {tokenRightBracket, "]"}, {tokenSemicolon, ";"}, {tokenRightBrace, "}"}, {tokenEOF, ""},
	}
	if len(tokens) != len(want) {
		t.Fatalf("tokens = %#v", tokens)
	}
	for index := range want {
		if tokens[index].Kind != want[index].kind || tokens[index].Text != want[index].text {
			t.Errorf("token %d = %#v; want %#v", index, tokens[index], want[index])
		}
	}
	if tokens[0].Position.Line != 2 || tokens[1].Position.Line != 3 {
		t.Fatalf("positions = %#v, %#v", tokens[0].Position, tokens[1].Position)
	}
}

func TestLexInstalledALSACorpus(t *testing.T) {
	path := os.Getenv("GO_ALSA_TEST_CONFIG")
	if path == "" {
		t.Skip("set GO_ALSA_TEST_CONFIG to an ALSA configuration file")
	}
	input, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := lex(string(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) < 2 || tokens[len(tokens)-1].Kind != tokenEOF {
		t.Fatalf("invalid token stream of length %d", len(tokens))
	}
	document, err := Parse(path, string(input))
	if err != nil {
		t.Fatal(err)
	}
	if value, err := document.ResolveReference("defaults.pcm.dmix.card"); err != nil || value.Kind != ValueInteger || value.Integer != 0 {
		t.Fatalf("resolve defaults.pcm.dmix.card = %#v, %v", value, err)
	}
	if value, err := NewEvaluator(document, EvalOptions{}).Reference("defaults.pcm.dmix.card"); err != nil || value.Kind != ValueInteger || value.Integer != 0 {
		t.Fatalf("evaluate defaults.pcm.dmix.card = %#v, %v", value, err)
	}
	t.Logf("lexed %d bytes into %d tokens and %d root statements", len(input), len(tokens), len(document.Statements))
}

func TestLexRejectsUnterminatedValues(t *testing.T) {
	for _, input := range []string{"x \"unterminated", "<confdir:file", "<>"} {
		_, err := lex(input)
		var syntax *syntaxError
		if !errors.As(err, &syntax) || syntax.Position.Line != 1 {
			t.Errorf("lex(%q) = %v", input, err)
		}
	}
}

func FuzzLex(f *testing.F) {
	for _, seed := range []string{"pcm.hw { type hw card 0 }", "<confdir:pcm/front.conf>", "@hooks [ { func load } ]", "x \"a\\nb\"", "# comment\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) { _, _ = lex(input) })
}
