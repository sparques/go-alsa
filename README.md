# go-alsa

`go-alsa` is an in-progress, Go-native implementation of Linux ALSA userspace
support. Production packages do not use cgo or dynamically load libasound.

The first implementation milestone covers direct kernel ALSA devices. See
[`plan.md`](plan.md) for the compatibility roadmap. APIs are unstable before
v1.0.

## Current status

The package can enumerate ALSA control devices and retrieve kernel card
metadata on Linux amd64 and arm64. PCM streaming, controls, mixers, ALSA
configuration evaluation, and software plugins are not implemented yet.

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
