package config

import (
	"errors"
	"strings"
	"testing"
)

func TestControlGraphMaterializesChildren(t *testing.T) {
	document, err := Parse("test", `ctl.default { type empty child direct }
ctl.direct { type hw card 3 }`)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := NewEvaluator(document, EvalOptions{}).ControlGraph("default")
	if err != nil || graph.Root.Type != "empty" || graph.Root.Child == nil || graph.Root.Child.Type != "hw" {
		t.Fatalf("control graph = %#v, %v", graph, err)
	}
}

func TestControlGraphReportsCycle(t *testing.T) {
	document, err := Parse("test", `ctl.a { type empty child b }
ctl.b { type empty child a }`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewEvaluator(document, EvalOptions{}).ControlGraph("a")
	if !errors.Is(err, ErrControlGraphCycle) || !strings.Contains(err.Error(), "a -> b -> a") {
		t.Fatalf("control cycle = %v", err)
	}
}

func FuzzControlGraph(f *testing.F) {
	for _, seed := range []string{
		`ctl.a { type hw card 0 }`,
		`ctl.a { type empty child b } ctl.b { type hw card 0 }`,
		`ctl.a { type empty child a }`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		document, err := Parse("fuzz", input)
		if err != nil {
			return
		}
		_, _ = NewEvaluator(document, EvalOptions{}).ControlGraph("a")
	})
}
