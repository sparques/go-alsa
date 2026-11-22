# Linux ALSA UAPI notes

The production package consumes checked-in Go representations of the Linux
userspace ABI and does not include C headers during normal builds.

## Source and licensing

The authoritative interface is Linux `include/uapi/sound/asound.h`, distributed
under `GPL-2.0+ WITH Linux-syscall-note`. Constants and binary structure layouts
in `internal/uapi` describe that public syscall interface. Package behavior is
implemented independently in Go.

The first checked-in definitions were verified against the installed Linux UAPI
header exposing PCM protocol 2.0.18 and control protocol 2.0.10. Each structure
has compile-time size assertions and ioctl-number tests. A later maintainer tool
will compare sizes, offsets, enum values, and ioctl numbers with pinned headers
on every supported architecture.

## Architecture policy

Only amd64 and arm64 are initially enabled. Both use 64-bit `long`, pointers,
and `time_t`, but definitions are still build-tagged because later PCM and
control structures contain architecture-sized members. New architectures must
not be enabled merely because they compile: their layouts and ioctl numbers
must pass the conformance probe described in `plan.md`.

## Initial inventory

The control discovery slice uses:

- `SNDRV_CTL_IOCTL_PVERSION` with a 32-bit integer.
- `SNDRV_CTL_IOCTL_CARD_INFO` with `snd_ctl_card_info`.

PCM discovery will next add `snd_pcm_info`,
`SNDRV_CTL_IOCTL_PCM_NEXT_DEVICE`, and `SNDRV_CTL_IOCTL_PCM_INFO`. Streaming
requires the substantially larger `snd_pcm_hw_params`, `snd_pcm_sw_params`,
transfer, status, sync-pointer, and channel-area structures. Those definitions
will be introduced with their corresponding behavioral tests rather than as an
untested bulk transcription.
