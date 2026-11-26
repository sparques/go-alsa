package config

import (
	"errors"
	"strings"
	"testing"
)

func TestPCMGraphMaterializesCompoundAndNamedSlaves(t *testing.T) {
	document, err := Parse("test", `pcm.default { type empty slave.pcm { type plug slave.pcm "hw:2,1" } hint.description Default }
pcm.hw {
 @args [ CARD DEV ]
 @args.CARD { type string }
 @args.DEV { type integer default 0 }
 type hw
 card $CARD
 device $DEV
}`)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := NewEvaluator(document, EvalOptions{}).Graph("default")
	if err != nil {
		t.Fatal(err)
	}
	if graph.Root.Type != "empty" || graph.Root.Slave == nil || graph.Root.Slave.Type != "plug" || graph.Root.Slave.Slave == nil || graph.Root.Slave.Slave.Type != "hw" {
		t.Fatalf("graph = %#v", graph.Root)
	}
	card := graph.Root.Slave.Slave.Fields["card"]
	device := graph.Root.Slave.Slave.Fields["device"]
	if card.String != "2" || device.Integer != 1 {
		t.Fatalf("hw fields = card %#v, device %#v", card, device)
	}
	if graph.Root.Fields["hint.description"].String != "Default" {
		t.Fatal("root fields were not preserved")
	}
}

func TestPCMGraphReportsResolutionChain(t *testing.T) {
	document, err := Parse("test", `pcm.a { type plug slave.pcm b }
pcm.b { type plug slave.pcm c }
pcm.c { type plug slave.pcm a }`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewEvaluator(document, EvalOptions{}).Graph("a")
	if !errors.Is(err, ErrGraphCycle) || !strings.Contains(err.Error(), "a -> b -> c -> a") {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestPCMGraphRetainsUnknownPlugin(t *testing.T) {
	document, err := Parse("test", `pcm.vendor { type third_party opaque.value 9 }`)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := NewEvaluator(document, EvalOptions{}).Graph("vendor")
	if err != nil || graph.Root.Type != "third_party" || graph.Root.Fields["opaque.value"].Integer != 9 {
		t.Fatalf("graph = %#v, %v", graph, err)
	}
}

func FuzzPCMGraph(f *testing.F) {
	for _, seed := range []string{
		`pcm.a { type hw card 0 }`,
		`pcm.a { type plug slave.pcm b } pcm.b { type hw card 0 }`,
		`pcm.a { type plug slave.pcm a }`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		document, err := Parse("fuzz", input)
		if err != nil {
			return
		}
		_, _ = NewEvaluator(document, EvalOptions{}).Graph("a")
	})
}
