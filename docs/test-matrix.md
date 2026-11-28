# Hardware and build test matrix

This file records reproducible evidence rather than implying that a checked
implementation item has passed every release environment.

## Current host report — 2026-11-28

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
- `snd-aloop`: deterministic device-0 playback to device-1 capture tests use
  the stable `Loopback` card ID and matching subdevice rather than a card
  number.
- `snd-dummy`: negotiation, prepared/running/drop/free lifecycle tests use the
  stable `Dummy` card ID.

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

Reproduce virtual-device tests after loading `snd-aloop` and `snd-dummy`:

```sh
GO_ALSA_TEST_ALOOP=1 go test ./pcm -run TestKernelLoopback -v
GO_ALSA_TEST_DUMMY=1 go test ./pcm -run TestDummyHardwareLifecycle -v
```

Override the stable defaults when needed with
`GO_ALSA_TEST_ALOOP_PLAYBACK_PCM`, `GO_ALSA_TEST_ALOOP_CAPTURE_PCM`, and
`GO_ALSA_TEST_DUMMY_PCM`.

## Automated build gates

- Native Linux amd64 tests with `CGO_ENABLED=0`.
- Race-enabled tests (the race runtime uses cgo; production packages do not).
- Linux arm64 cross-tests with `CGO_ENABLED=0`.
- FreeBSD amd64 stub build.
- `go vet ./...` and `git diff --check`.

Still missing for release qualification: Linux 4.14 execution, arm64 hardware,
no-device/permission containers, long-duration loopback,
and the additional architectures listed in the implementation plan.

