# Compatibility policy

## Supported build targets

The release-blocking targets are Linux amd64 and arm64 with `CGO_ENABLED=0`.
They share the checked 64-bit ALSA layouts in `internal/uapi`; both are built
and tested on every development gate. Other operating systems build small
stubs that return `alsa.ErrUnsupportedPlatform`.

The module currently requires Go 1.27.3, as declared by `go.mod`. Lower Go
versions are not supported unless the module directive and full test matrix are
deliberately lowered together.

Linux 4.14 is the intended minimum kernel baseline. The syscalls used by the
descriptor and cancellation layer predate it, and every optional ALSA behavior
is negotiated through protocol versions, capability flags, or ioctl results.
Release qualification on the minimum kernel is still required; the current
hardware report uses Linux 6.18.58.

## Device-name and feature boundary

| Area | Current behavior |
| --- | --- |
| `hw:` PCM names | Numeric cards and stable card IDs, device, subdevice, and direction selectors |
| Direct PCM | Playback/capture, interleaved and non-interleaved read/write, common integer formats, exact/near parameters, polling, lifecycle, XRUN/suspend recovery |
| `plughw:`, `default`, aliases | Not yet implemented; returns a precise parse/resolution error rather than silently treating the name as `hw:` |
| mmap PCM | Not yet implemented |
| Raw controls | Full element identities, common value types, TLV, events, power, locks, and user elements |
| Simple mixer | Conventional simple-none grouping, volume/switch/dB/enumerated behavior; broader differential fixtures remain |
| libasound plugins | Not loaded; configuration, conversion, routing, sharing, and sound-server integration remain separate phases |

The library communicates only with kernel ALSA device nodes. PipeWire,
PulseAudio, JACK, Bluetooth services, and binary libasound plugins are not
reachable through ALSA kernel ioctls alone.

