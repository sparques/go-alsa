# Troubleshooting direct ALSA access

## Permission denied

`errors.Is(err, alsa.ErrPermission)` identifies device-node permission errors
while the original `EACCES` or `EPERM` remains available. Check the ownership
and ACL of `/dev/snd/*`, the session's audio-device policy, and container device
mapping. The package never changes permissions itself.

## Device busy

Direct `hw:` devices are commonly exclusive. `errors.Is(err,
alsa.ErrDeviceBusy)` identifies `EBUSY`. Close other direct clients or select a
different subdevice. Desktop PipeWire/PulseAudio sessions may own hardware;
this package does not bypass or impersonate those servers.

## Device disappeared

Open-time absence matches `alsa.ErrDeviceNotFound`. Removal after open matches
`alsa.ErrDisconnected`, and the underlying `ENODEV`/`ENXIO` remains testable.
Close the handle, enumerate again, and resolve the stable card ID rather than
assuming the old numeric card index survived hotplug.

## Hardware parameters rejected

Use `Handle.Capabilities` to inspect ranges. `ConfigureHardware` requires exact
values; `ConfigureHardwareNear` keeps access, format, and channels exact while
choosing jointly supported rate, period count/size, and buffer size. Always use
the returned `HardwareSetup`, not the requested values, for buffer arithmetic.

## XRUN or suspend

Strict recovery is the default. XRUN errors match both `pcm.ErrXRUN` and
`EPIPE`; suspended streams match both `pcm.ErrSuspended` and `ESTRPIPE`.
`RecoveryAutomatic` prepares after XRUN and attempts resume—with a prepare
fallback—after suspend. Inspect `Handle.Stats` when diagnosing recurring
overruns, underruns, or suspensions.

## Containers

The container must receive the needed `/dev/snd/controlC*` and PCM nodes.
Discovery cannot see host devices that were not mapped into its device
namespace. Seccomp policies must permit `ioctl`, `ppoll`, and `eventfd2`.

