package pcm

import (
	"errors"
	"testing"

	alsa "github.com/sparques/go-alsa"
	alsaconfig "github.com/sparques/go-alsa/config"
)

const namedPlanConfig = `pcm.hw {
 @args [ CARD DEV ]
 @args.CARD { type string }
 @args.DEV { type integer default 0 }
 type hw card $CARD device $DEV
}
pcm.plughw {
 @args [ CARD DEV ]
 @args.CARD { type string }
 @args.DEV { type integer default 0 }
 type plug slave.pcm { type hw card $CARD device $DEV }
}
pcm.duplex {
 type empty
 slave.pcm {
  type asym
  playback.pcm "plughw:3,1"
  capture.pcm "hw:3,2"
 }
}`

func TestPlanNamedSelectsDirectionAndPlug(t *testing.T) {
	document, err := alsaconfig.Parse("test", namedPlanConfig)
	if err != nil {
		t.Fatal(err)
	}
	evaluator := alsaconfig.NewEvaluator(document, alsaconfig.EvalOptions{})
	playback, err := PlanNamed(evaluator, "duplex", alsa.Playback)
	if err != nil {
		t.Fatal(err)
	}
	if !playback.RequiresPlug || playback.HardwareSelector != "hw:CARD=3,DEV=1" || len(playback.Plugins) != 4 {
		t.Fatalf("playback plan = %#v", playback)
	}
	capture, err := PlanNamed(evaluator, "duplex", alsa.Capture)
	if err != nil {
		t.Fatal(err)
	}
	if capture.RequiresPlug || capture.HardwareSelector != "hw:CARD=3,DEV=2" || len(capture.Plugins) != 3 {
		t.Fatalf("capture plan = %#v", capture)
	}
}

func TestNamedPlanBuildsConverterAfterNegotiation(t *testing.T) {
	document, _ := alsaconfig.Parse("test", namedPlanConfig)
	plan, err := PlanNamed(alsaconfig.NewEvaluator(document, alsaconfig.EvalOptions{}), "plughw:3,0", alsa.Playback)
	if err != nil {
		t.Fatal(err)
	}
	application := ConversionParams{Format: U8, Channels: 1, Rate: 16000}
	hardware := ConversionParams{Format: S16LE, Channels: 2, Rate: 48000}
	converter, err := plan.Converter(application, hardware, nil)
	if err != nil || converter == nil || converter.ApplicationParams() != application || converter.HardwareParams() != hardware {
		t.Fatalf("Converter = %#v, %v", converter, err)
	}
	direct, err := PlanNamed(alsaconfig.NewEvaluator(document, alsaconfig.EvalOptions{}), "hw:3,0", alsa.Playback)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := direct.Converter(application, hardware, nil); !errors.Is(err, ErrConversionRequired) {
		t.Fatalf("direct conversion error = %v", err)
	}
	if converter, err := direct.Converter(hardware, hardware, nil); err != nil || converter != nil {
		t.Fatalf("matching direct converter = %#v, %v", converter, err)
	}
}

func TestPlanNamedRejectsExecutablePlugin(t *testing.T) {
	document, err := alsaconfig.Parse("system.conf", `pcm.default { type dmix ipc_key 1 }`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = PlanNamed(alsaconfig.NewEvaluator(document, alsaconfig.EvalOptions{}), "default", alsa.Playback)
	if !errors.Is(err, ErrUnsupportedPlugin) {
		t.Fatalf("PlanNamed error = %v", err)
	}
}
