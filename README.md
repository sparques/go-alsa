# go-alsa

`go-alsa` is an in-progress, Go-native implementation of Linux ALSA userspace
support. Production packages do not use cgo or dynamically load libasound.

The first implementation milestone covers direct kernel ALSA devices. See
[`plan.md`](plan.md) for the compatibility roadmap. APIs are unstable before
v1.0.

## Current status

The package can enumerate ALSA cards and playback/capture PCM devices on Linux
amd64 and arm64. It also provides direct-hardware PCM streaming, typed raw
control access, control events and TLVs, and an initial simple-mixer layer.
ALSA configuration evaluation and software plugins are not implemented yet.

```go
cards, err := alsa.Cards()
if err != nil {
    log.Fatal(err)
}
for _, card := range cards {
    fmt.Printf("%d: %s (%s)\n", card.Number, card.Name, card.ID)
}
```

Linux amd64 and arm64 are the initial release-blocking architectures.
See [compatibility](docs/compatibility.md), [troubleshooting](docs/troubleshooting.md),
and the current [hardware/build test matrix](docs/test-matrix.md) for precise
support boundaries and reproducible evidence.

Run `go run ./cmd/alsa-info-go` to inspect the cards and PCM devices reported by
the kernel.

`alsa-pcm-go` is a headerless raw-PCM capture/playback smoke tool built entirely
on the public Go API. For example, capture five seconds of mono 48 kHz S16_LE:

```sh
go run ./cmd/alsa-pcm-go -device hw:3,0 -capture -channels 1 -duration 5s > capture.raw
```

Play a stereo raw stream through a device with exact parameters:

```sh
go run ./cmd/alsa-pcm-go -device hw:3,0 -channels 2 -exact < playback.raw
```

The direction-specific `go-arecord` and `go-aplay` commands provide the same
raw streaming engine with conventional command names.

Inspect simple mixer and enumerated controls without libasound:

```sh
go run ./cmd/go-amixer -card Device
go run ./cmd/go-amixer -card Device -name Mic -capture-volume 0.5
```

Direct hardware names can be parsed and resolved without libasound:

```go
device, err := alsa.ResolveHardwarePCM("hw:CARD=Generic,DEV=0", alsa.Capture)
if err != nil {
    log.Fatal(err)
}
fmt.Println(device.PCM.Path)
```

Parsed configuration can also open named definitions that evaluate directly to
`type hw`; other plugin types remain explicit until their graph implementation
is available:

```go
document, err := config.Parse("application.conf", `pcm.radio { type hw card Device device 0 }`)
if err != nil {
    log.Fatal(err)
}
handle, err := pcm.OpenNamed(config.NewEvaluator(document, config.EvalOptions{}), "radio", alsa.Capture)
```

Conventional kernel mixer controls are grouped by their Playback/Capture
Switch/Volume suffixes. Raw values and decoded hundredths-of-a-decibel values
are both available:

```go
handle, err := mixer.Open("3")
if err != nil {
    log.Fatal(err)
}
defer handle.Close()

elements, err := handle.Elements()
if err != nil {
    log.Fatal(err)
}
for _, element := range elements {
    if element.HasPlaybackVolume() {
        volume, err := element.PlaybackVolume()
        if err != nil {
            log.Fatal(err)
        }
        fmt.Printf("%s: raw=%v dB=%v\n", element.Name, volume.Values, volume.DB)
    }
}
```

Hardware tests are opt-in. For a direct PCM with its output cabled to its input,
run the analog loopback test with:

```sh
GO_ALSA_TEST_LOOPBACK_PCM=hw:3,0 go test ./pcm -run TestHardwareLoopback -v
```

The current test supports asymmetric stereo-playback/mono-capture USB adapters,
transfers one second at 48 kHz, checks spectral dominance of the looped-back
1 kHz tone, and requires zero reported XRUNs.
