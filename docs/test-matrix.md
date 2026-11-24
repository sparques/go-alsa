# Hardware and build test matrix

This file records reproducible evidence rather than implying that a checked
implementation item has passed every release environment.

## Current host report — 2026-11-24

- Host: Linux 6.18.58 x86_64, Go 1.27.3, cgo disabled for library tests.
- HDA ATI HDMI: two playback PCMs discovered.
- HD-Audio Generic ALC897: analog playback/capture, digital playback, and
  alternate analog capture discovered; direct open/configuration and
  control/mixer operations exercised during development.
- USB Camera: capture PCM discovered.
- C-Media USB PnP Sound Device: playback/capture discovered; exact and near
  configuration, timestamps, lifecycle, transfer, XRUN behavior, control and
  mixer operations exercised.
- C-Media analog loopback: simultaneous stereo playback and mono capture for
  one second at 48 kHz; dominant 1 kHz tone and zero XRUN counters.

`alsa-info-go` ordering and metadata matched `aplay -l` and `arecord -l` for all
four cards and nine stream directions visible during this report.

Reproduce the discovery comparison with:

```sh
aplay -l
arecord -l
go run ./cmd/alsa-info-go
```

Reproduce the connected USB loopback with:

```sh
GO_ALSA_TEST_LOOPBACK_PCM=hw:3,0 go test ./pcm -run TestHardwareLoopback -v
```

## Automated build gates

- Native Linux amd64 tests with `CGO_ENABLED=0`.
- Race-enabled tests (the race runtime uses cgo; production packages do not).
- Linux arm64 cross-tests with `CGO_ENABLED=0`.
- FreeBSD amd64 stub build.
- `go vet ./...` and `git diff --check`.

Still missing for release qualification: Linux 4.14 execution, arm64 hardware,
virtual ALSA devices, no-device/permission containers, long-duration loopback,
and the additional architectures listed in the implementation plan.

