# go-alsa

`go-alsa` is an in-progress, Go-native implementation of Linux ALSA userspace
support. Production packages do not use cgo or dynamically load libasound.

The first implementation milestone covers direct kernel ALSA devices. See
[`plan.md`](plan.md) for the compatibility roadmap. APIs are unstable before
v1.0.

## Current status

The package can enumerate ALSA control devices, retrieve kernel card metadata,
and enumerate playback/capture PCM devices on Linux amd64 and arm64. PCM
streaming, controls, mixers, ALSA configuration evaluation, and software
plugins are not implemented yet.

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

Run `go run ./cmd/alsa-info-go` to inspect the cards and PCM devices reported by
the kernel.

Direct hardware names can be parsed and resolved without libasound:

```go
device, err := alsa.ResolveHardwarePCM("hw:CARD=Generic,DEV=0", alsa.Capture)
if err != nil {
    log.Fatal(err)
}
fmt.Println(device.PCM.Path)
```
