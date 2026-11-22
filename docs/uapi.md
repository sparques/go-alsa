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

Run the current-host comparison with:

```sh
go run ./tools/abi-probe
```

The tool compiles an embedded C probe in a temporary directory, runs it, and
compares its results with `internal/uapi`. It is a maintainer check only: the
library and its normal test/build path remain independent of a C compiler and
system ALSA headers.

## Architecture policy

Only amd64 and arm64 are initially enabled. Both use 64-bit `long`, pointers,
and `time_t`, but definitions are still build-tagged because later PCM and
control structures contain architecture-sized members. New architectures must
not be enabled merely because they compile: their layouts and ioctl numbers
must pass the conformance probe described in `plan.md`.

## Initial inventory

The control and PCM discovery slice uses:

- `SNDRV_CTL_IOCTL_PVERSION` with a 32-bit integer.
- `SNDRV_CTL_IOCTL_CARD_INFO` with `snd_ctl_card_info`.
- `SNDRV_CTL_IOCTL_PCM_NEXT_DEVICE` to walk kernel PCM device numbers.
- `SNDRV_CTL_IOCTL_PCM_INFO` with `snd_pcm_info` for each stream direction.
- `SNDRV_PCM_IOCTL_PVERSION` and `SNDRV_PCM_IOCTL_INFO` when opening a direct
  kernel PCM handle.

Streaming requires the substantially larger `snd_pcm_hw_params`, `snd_pcm_sw_params`,
transfer, status, sync-pointer, and channel-area structures. Those definitions
will be introduced with their corresponding behavioral tests rather than as an
untested bulk transcription.

The first streaming-related definition, `snd_pcm_hw_params`, is now checked for
size and critical offsets and used for exact S16_LE interleaved hardware
negotiation. The remaining streaming ABI is being introduced alongside each
behavioral implementation rather than as an untested bulk transcription.

Software parameters, prepare/start, and interleaved transfer ioctls are now
represented and checked as well. The first transfer API is deliberately strict:
it reports XRUNs for explicit recovery instead of silently preparing the stream.
Synchronization pointers, non-interleaved access, and mmap channel areas remain
pending. Kernel status and delay ioctls are represented and used by the public
PCM API. State-aware playback drain uses those snapshots to distinguish
DRAINING, completion in SETUP, XRUN, and disconnection; this avoids treating
ALSA's device-specific `POLLERR` drain notification as a generic I/O failure.
