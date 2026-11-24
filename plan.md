# go-alsa implementation plan

## Progress tracker

This is the canonical cross-session status list. The detailed sections below
describe the requirements and exit criteria; check an item here only when its
implementation and proportionate tests are present. A checked implementation
item does not imply that its phase exit criteria have been met.

- [ ] **Phase 0 — research and executable specifications**
  - [x] Record the Linux UAPI source, licensing exception, initial protocol
    versions, and architecture policy in `docs/uapi.md`.
  - [ ] Complete the PCM/control UAPI inventory and compatibility matrices.
  - [ ] Add libasound reference probes and normalized comparison fixtures.
  - [x] Select and document minimum Go and Linux kernel versions.
- [ ] **Phase 1 — Linux UAPI and ioctl foundation**
  - [x] Implement Linux ioctl encoding and compile-time/runtime layout checks
    for the initial control and PCM discovery structures.
  - [x] Check that the current amd64 implementation and arm64 layouts build
    with `CGO_ENABLED=0`.
  - [ ] Pin source headers and add the maintainer ABI generator/conformance
    probe for every supported architecture. (A current-host conformance probe
    now covers the checked-in discovery ABI.)
  - [x] Add a race-safe descriptor wrapper with deterministic open, ioctl,
    read, write and close syscall seams.
  - [x] Make ppoll/eventfd creation injectable and test cancellation both
    deterministically and against real nonblocking pipes.
  - [x] Add context cancellation using `ppoll` and eventfd wakeups, including
    close-triggered wakeup and concurrency tests.
  - [ ] Meet the complete Phase 1 exit criteria on amd64 and arm64.
- [ ] **Phase 2 — discovery and direct hardware resolution**
  - [x] Enumerate `/dev/snd/controlC*` and query card metadata with kernel
    ioctls rather than parsing `/proc/asound`.
  - [x] Enumerate playback/capture PCM directions, subdevice counts, and
    availability through control ioctls.
  - [x] Return deterministically ordered public card/PCM metadata and tolerate
    a control device disappearing between discovery and open.
  - [x] Parse and resolve numeric and stable-ID `hw:` names, including device,
    subdevice, and direction selectors.
  - [x] Add typed errors for permissions, absent/busy devices, malformed names,
    unsupported directions, and disconnection while preserving underlying
    syscall errors for `errors.Is`.
  - [ ] Compare enumeration with `aplay`, `arecord`, and libasound on the full
    virtual/real hardware matrix. (The current amd64 host agrees with `aplay -l`
    and `arecord -l` across HDA/HDMI, onboard analog/digital, USB camera, and
    USB C-Media devices; virtual and arm64 hardware remain.)
- [ ] **Phase 3 — direct PCM engine**
  - [x] Open a resolved direct PCM nonblocking, check its protocol major, query
    stream metadata, enforce direction, and close it idempotently. An opt-in
    hardware test covers this path without configuring or transferring audio.
  - [ ] Implement hardware/software parameter negotiation and streaming.
    (Exact S16_LE, RW-interleaved hardware constraints, software defaults, and
    prepare are implemented; initial nonblocking interleaved transfers and
    strict/automatic XRUN recovery, status/delay queries, and state-aware drain
    are implemented; reset, pause/resume, rewind/forward, and hardware sync are
    exposed; unconstrained access/format/interval capability discovery is
    implemented; byte-oriented transfers support common 8/16/24/32-bit integer
    layouts with interleaved and non-interleaved RW access, while typed
    transfers remain available for interleaved S16_LE; software timestamp mode
    and realtime/monotonic clock selection, available-frame queries, and
    cancellation-aware readiness polling are exposed; strict and automatic
    suspend recovery preserve `ESTRPIPE` across direct ioctl, readiness-poll,
    and drain paths; available-frame hardware timestamps come from one status
    snapshot; reset/pause/resume/pointer/sync operations validate configuration
    and pause/resume capabilities; runtime `ENODEV`/`ENXIO` failures preserve
    their errno while classifying as disconnection; kernel stream link/unlink is exposed
    with direction and lifecycle validation; near negotiation selects jointly
    supported rate, period size/count, and buffer sizes while preserving exact
    access/format/channels, and negotiated timing/rational-rate/sync metadata is
    retained; broader negotiation and remaining lifecycle
    operations remain.)
- [ ] **Phase 4 — raw control API**
  - [x] Negotiate the control protocol during open and expose typed card info
    with complete component metadata.
  - [x] Open controls by numeric card or stable ID, enumerate full element IDs,
    and query typed metadata, access flags, integer ranges, and enum item names.
  - [x] Read and validate/write boolean, integer, integer64, enumerated, bytes,
    and IEC958 values.
  - [x] Implement element locks and bounded raw TLV read/write/command support.
  - [x] Decode standard dB scale/linear/min-max/range, container, and channel-map
    TLVs with nesting and bounds checks.
  - [x] Implement cancellation-aware ordered control event subscriptions.
  - [x] Implement typed card power-state queries and validated state requests.
  - [x] Implement validated user-element add/replace/remove for boolean,
    integer, integer64, enumerated, byte, and IEC958 controls.
  - [x] Stress ordered ADD/VALUE/REMOVE events and enumeration with 64 temporary
    userspace controls under bounded cancellation and guaranteed cleanup.
  - [x] Read full card component strings with bounded CARD_BYTES allocation,
    change retries, and an explicit legacy-kernel completeness fallback.
  - [x] Complete the card/control-element ioctl inventory. HWDEP, RawMIDI, UMP,
    and preferred-subdevice ioctls are separate device-discovery extensions.
- [ ] **Phase 5 — simple mixer layer**
  - [x] Group standard playback/capture switch and volume controls by base name
    and index; provide typed reads and validated writes.
  - [x] Convert dB scale, linear-amplitude, min/max, mute, and range TLVs to
    alsa-lib-compatible hundredths-of-dB values.
  - [x] Add bounded, step-aware normalized raw-volume APIs.
  - [x] Forward cancellation-aware control events with simple-element
    direction and kind classification while retaining unknown raw events.
  - [x] Expose vendor-named enumerated mixer controls, item names, per-channel
    selections and validated writes without guessing their direction.
  - [x] Cache capability discovery with defensive copies, explicit refresh,
    uncached values, and INFO/ADD/REMOVE/TLV event invalidation.
  - [x] Match simple-none playback/capture/global suffix and unsuffixed-control
    grouping; expose alsa-lib channel identities plus common/joined capabilities.
  - [x] Classify enumerated directions and identify per-channel exclusive
    Capture Source selection groups without guessing vendor-specific names.
  - [ ] Add broader hardware fixtures.
- [ ] **Phase 6 — ALSA configuration language**
- [ ] **Phase 7 — software PCM graph and conversion**
- [ ] **Phase 8 — shared PCM plugins**
- [ ] **External sound-server detection and extension boundary**
- [ ] **Public API design and stability review**
- [ ] **Testing strategy complete**
  - [x] Add initial ioctl/layout, parsing, and hardware-optional discovery
    tests.
  - [x] Pass native `CGO_ENABLED=0` tests, arm64 cross-build tests, non-Linux
    stub builds, race-enabled tests, `go vet`, and `git diff --check` for the
    initial slices. The race detector uses cgo instrumentation, but the library
    itself remains cgo-free.
  - [ ] Complete the planned discovery and streaming hardware matrix. Hardware
    tests run through explicit host-access approval because the normal agent
    sandbox does not expose `/dev/snd`.
  - [x] Run an opt-in one-second analog loopback on the C-Media USB card using
    simultaneous stereo playback and mono capture; verify the expected tone is
    dominant and both stream XRUN counters remain zero.
  - [ ] Add race, fuzz, differential, virtual-device, and hardware matrices.
- [ ] **Performance and realtime requirements complete**
- [ ] **Security and robustness review complete**
- [ ] **Documentation and examples complete**
  - [x] Add initial package README, API documentation, UAPI notes, and the
    `alsa-info-go` kernel discovery command.
  - [ ] Add PCM/control/mixer examples, compatibility and troubleshooting
    documentation, migration guide, `go-amixer`, `go-aplay`, and `go-arecord`.
    (`alsa-pcm-go` now provides a raw direct-hardware capture/playback example.)
- [ ] **v0.1 release gate — direct kernel foundation**
- [ ] **v0.2 release gate — complete direct PCM and controls**
- [ ] **v0.3 release gate — configuration and mixer**
- [ ] **v0.4 release gate — plug and routing**
- [ ] **v0.5 release gate — shared devices**
- [ ] **v1.0 release gate**
- [ ] **Downstream Vulpes integration and validation**
- [ ] **Project management and evidence requirements continuously enforced**

## 1. Purpose and definition of done

Build a Linux-only, Go-native ALSA package that talks directly to the kernel
ALSA UAPI. The package must not use cgo, dynamically load `libasound`, invoke
ALSA command-line tools, or require ALSA development headers at build time.

This is not intended to be only a thin `/dev/snd/pcm*` wrapper. The completed
package should provide the parts of the libasound experience needed by normal
applications:

- PCM capture and playback through blocking and nonblocking APIs.
- Hardware and software parameter negotiation, streaming, polling, XRUN
  recovery, draining, pausing, and clean cancellation.
- Card, device, PCM, and control enumeration with stable identifiers.
- Control-element discovery, metadata, events, and typed reads/writes.
- A simple-mixer model sufficient for playback/capture volume and switches.
- ALSA configuration parsing and named-device resolution, including `default`,
  aliases, user configuration, and a documented set of PCM plugins.
- Software conversion where a named PCM promises format, channel, or rate
  adaptation.
- Portable builds for the supported Linux architectures and useful behavior
  when cross-compiling with `CGO_ENABLED=0`.
- Differential and hardware tests demonstrating compatibility with libasound.

The definition of done is that a downstream program can replace its libasound
PCM and control usage with this module, retain common ALSA device names, and
build a static Go binary without cgo. Any unsupported libasound feature must
produce a precise error rather than silently changing semantics.

## 2. Scope and compatibility contract

### Initial supported platforms

- Linux amd64 and arm64 are release-blocking.
- Linux 386, arm, riscv64, ppc64le, and s390x are supported after their ABI
  layouts pass generated conformance tests.
- Other operating systems receive compile-time stubs returning
  `ErrUnsupportedPlatform` only if that materially improves downstream build
  ergonomics; otherwise constrain files with Linux build tags.

### PCM baseline

- Playback and capture.
- Interleaved and non-interleaved access.
- Integer PCM formats commonly exposed by USB and onboard codecs, beginning
  with S16_LE and expanding to the standard 8-, 16-, 24-, and 32-bit formats.
- One or more channels, exact and near-rate negotiation, configurable period
  and buffer sizing, timestamps, available/delay queries, poll descriptors,
  mmap when safely supported, and read/write fallback.
- Correct state transitions for OPEN, SETUP, PREPARED, RUNNING, XRUN, DRAINING,
  PAUSED, SUSPENDED, and DISCONNECTED.

### Named devices and plugins

Classify compatibility explicitly:

1. **Kernel-backed:** `hw`, card/control devices, and direct PCM access.
2. **Parameter-adapting:** `plughw` and `plug`, implemented by package-owned
   conversion stages.
3. **Routing/composition:** `route`, `copy`, `null`, `file`, `tee`, and similar
   plugins selected after studying actual configuration use.
4. **Shared/concurrent:** `dmix`, `dsnoop`, and `dshare`, which require IPC,
   shared-memory, locking, clocking, and multi-client tests.
5. **Server/external:** PulseAudio, PipeWire, JACK, Bluetooth, and arbitrary
   third-party plugins. These are not kernel ALSA protocols. Support requires
   separately scoped native clients or an explicit extension mechanism.

Do not claim complete libasound plugin compatibility until every plugin class
has a tested implementation. Preserve unknown configuration nodes so errors can
identify the unsupported plugin and its source location.

### Control and mixer baseline

- Enumerate cards and control elements.
- Read control metadata and values for boolean, integer, integer64, enumerated,
  bytes, IEC958, and TLV controls.
- Write eligible values and subscribe to control events.
- Resolve elements by full kernel identity: interface, device, subdevice, name,
  and index.
- Build simple-mixer elements by reproducing the relevant libasound grouping,
  channel, range, dB/TLV, switch, and joined-channel rules.
- Treat the mixer layer as a separate package over the raw control API so users
  can bypass policy when exact control access is required.

### Explicit non-goals for the first stable release

- Sequencer, timer, raw MIDI, topology, UCM, and compressed-offload APIs.
- ABI compatibility with libasound's C types.
- Loading binary libasound plugins.
- Pretending that PipeWire or PulseAudio endpoints are reachable solely through
  ALSA kernel ioctls.

Track these as future packages only after PCM and control APIs are stable.

## 3. Repository and package layout

Use a small public surface and keep Linux ABI machinery internal. A provisional
layout is:

```text
go.mod
alsa.go                  shared errors, version and discovery entry points
pcm/                     public PCM API and software pipeline
control/                 public raw control API
mixer/                   public simple-mixer abstraction
config/                  ALSA configuration lexer, parser, evaluator
internal/uapi/           generated ioctl constants and ABI structs
internal/device/         /dev/snd and /proc/asound discovery/resolution
internal/poll/           cancellation-aware ppoll/eventfd support
internal/convert/        sample format, channels and rate conversion
internal/plugin/         named PCM graph and built-in plugins
cmd/alsa-info-go/        diagnostic enumeration utility
cmd/alsa-pcm-go/         capture/playback smoke utility
testdata/                configurations, golden data and recorded metadata
tools/                   checked-in ABI generation and verification tooling
```

Avoid exposing raw ioctl structures from public packages. Public interfaces
should use Go-sized values, duration/rate types, typed enums, and errors that
support `errors.Is`/`errors.As`.

## 4. Phase 0: research and executable specifications

- Inventory the Linux ALSA UAPI headers under `include/uapi/sound` and document
  every ioctl, structure, union, flexible array, bitmap, pointer, `long`, and
  timespec needed by PCM and controls.
- Trace libasound's implementations of PCM hw/sw parameter setup, state
  recovery, device-name parsing, configuration evaluation, `plug`, rate
  conversion, `dmix`/`dsnoop`, and simple mixer elements.
- Record relevant licensing and provenance. Reimplement behavior from published
  interfaces and clean Go code; do not copy incompatible source into the module.
- Capture reference traces from a small C/libasound probe for representative
  devices and configurations. Store normalized ioctl sequences and negotiated
  results as test fixtures where licensing and machine specificity permit.
- Write compatibility matrices for kernel versions, architectures, formats,
  access modes, device strings, plugin types, and mixer operations.
- Decide the minimum supported Go and Linux kernel versions before publishing
  the first API.

Deliverable: design notes plus probe programs that turn compatibility claims
into repeatable tests.

## 5. Phase 1: Linux UAPI and ioctl foundation

- Generate Go constants and structure definitions from pinned Linux UAPI
  headers. Check generated output into the repository so normal builds need no
  C compiler or headers.
- Generate per-architecture layouts where `unsigned long`, pointers, alignment,
  endianness, or timespec layout differs. Never assume amd64 layout elsewhere.
- Implement ioctl-number construction and compile-time/runtime size assertions.
- Add a maintainer-only conformance generator that compiles tiny C probes and
  compares `sizeof`, `offsetof`, enum values, and ioctl numbers against Go.
- Run those probes in CI for every architecture available natively or through
  a trusted cross/QEMU job.
- Wrap file descriptors with deterministic, idempotent closure; preserve errno
  with `%w`; handle `EINTR`; and prevent operations from racing with close.
- Implement cancellation-aware polling using `ppoll` plus an eventfd/pipe so a
  canceled context and `Close` wake blocked operations immediately.
- Add syscall-injection seams for deterministic unit tests without hardware.

Exit criteria: all required ABI layouts match the pinned headers on amd64 and
arm64; race tests show that I/O, cancellation, and close cannot deadlock or use
a recycled descriptor.

## 6. Phase 2: discovery and direct hardware resolution

- Enumerate `/dev/snd/controlC*` and query kernel card/device metadata rather
  than treating `/proc/asound` text as authoritative.
- Use `/proc/asound` only for useful aliases or diagnostics not exposed by the
  control API, and tolerate it being absent in containers.
- Resolve numeric cards, stable card IDs, PCM device/subdevice numbers, and
  directions. Support `hw:0,0`, `hw:CARD=Radio,DEV=0`, and equivalent structured
  Go selectors.
- Return structured device information including card ID/name, PCM ID/name,
  capture/playback direction, subdevice availability, and `/dev/snd` path.
- Define deterministic ordering and behavior for hotplug/disappearance.
- Distinguish permission errors, absent devices, busy devices, malformed names,
  unsupported directions, and disconnected hardware.

Exit criteria: enumeration agrees with `aplay -l`, `arecord -l`, and libasound
probes across USB audio, onboard audio, virtual ALSA devices, and no-device
systems.

## 7. Phase 3: direct PCM engine

- Implement `Open`, protocol/version checks, info queries, and direction-safe
  capture/playback handles.
- Build hw-parameter masks and intervals without relying on C bitfields or
  host-sized assumptions.
- Negotiate access, format, subformat, channels, rate, period size/time/count,
  buffer size/time, and significant bits. Return the actual negotiated setup.
- Configure sw parameters including start/stop thresholds, avail minimum,
  silence behavior, timestamps, boundary values, and transfer alignment where
  supported by the kernel protocol.
- Implement prepare, start, drop, drain, pause, resume, reset, status, delay,
  rewind/forward, link/unlink, and htimestamp operations with state validation.
- Implement interleaved and vector read/write paths. Correctly handle partial
  frames, `EAGAIN`, `EINTR`, XRUN (`EPIPE`), suspend (`ESTRPIPE`), disconnect,
  short buffers, and device removal.
- Add mmap only after validating channel-area offsets, ring-boundary arithmetic,
  memory barriers, appl/hw pointers, and 32-bit wraparound. Keep read/write as a
  complete supported path.
- Define recovery policy separately from primitive operations. Allow automatic
  recovery with counters and callbacks, or strict mode that returns state
  errors to the caller.
- Expose stream statistics without making atomic implementation details public.
- Ensure no syscall retains a pointer to movable Go memory beyond its duration.

Exit criteria: long-running loopback is bit-exact for supported integer formats;
latency and XRUN behavior match requested thresholds; all blocking operations
cancel promptly; `go test -race` passes under concurrent close and status calls.

## 8. Phase 4: raw control API

- Implement card info, element list, element info, element read/write, lock,
  unlock, replace/add/remove where safe, TLV read/write/command, power state,
  subscription, and event reads.
- Represent full control IDs and typed values without lossy string shortcuts.
- Validate count, range, step, enum item, byte length, permissions, ownership,
  volatility, inactivity, and TLV capabilities before writes.
- Decode dB scale, dB linear, min/max, container, channel-map, and nested TLVs
  with strict bounds and overflow checks.
- Provide event streams that integrate with contexts, poll correctly, surface
  overflow/disconnection, and terminate on close.
- Test large element lists and controls changing between list and info calls.

Exit criteria: metadata and values compare field-for-field with `amixer cget`
and libasound probes for representative hardware; boolean GPIO-style controls
can be safely used for PTT without cgo.

## 9. Phase 5: simple mixer layer

- Document the exact libasound simple-element behavior being emulated.
- Group raw controls into playback/capture mixer elements by name, index,
  direction, channel, and suffix conventions.
- Implement per-channel and joined playback/capture switches and volumes,
  capture exclusivity, enum controls, raw ranges, normalized ranges, and dB
  conversion through TLVs.
- Preserve independent raw-control access for unusual codecs that do not fit
  simple-element conventions.
- Subscribe to control events and keep cached capabilities/values coherent
  across changes and hotplug.
- Build fixture-driven tests from multiple USB codecs and HDA cards, including
  missing channels, joined switches, non-linear dB ranges, and controls whose
  names collide at different indices.

Exit criteria: common mute and volume operations agree with `amixer sget/sset`
and libasound on the compatibility hardware set.

## 10. Phase 6: ALSA configuration language

- Implement a lexer and parser for the relevant ALSA configuration grammar,
  retaining filenames, line/column locations, includes, comments where useful,
  compound nodes, arrays, strings, integers, reals, references, and functions.
- Load system and user configuration according to documented ALSA precedence,
  with explicit options for hermetic applications. Respect environment-driven
  paths only through a clearly testable loader policy.
- Add include-cycle detection, recursion/depth limits, file-size limits, and
  secure handling of untrusted paths. Do not execute shell commands or load
  shared objects during evaluation.
- Implement variable substitution, references, merges/overrides, argument
  definitions, defaults, and the subset of built-in functions required by the
  shipped ALSA configurations on supported distributions.
- Resolve `pcm.NAME` and `ctl.NAME` into typed graphs. Detect graph cycles and
  report the complete resolution chain with source locations.
- Test against ALSA configuration trees from Debian/Ubuntu, Fedora, Arch,
  Raspberry Pi OS, and common embedded distributions, subject to fixture
  licensing.

Exit criteria: `default`, named cards, and common distribution aliases resolve
to equivalent typed graphs without linking or executing libasound.

## 11. Phase 7: software PCM graph and conversion

- Define a private pull/push graph abstraction for hardware endpoints,
  converters, routers, buffers, and fan-out nodes. Keep realtime paths free of
  unbounded allocation and avoid goroutine-per-sample-stage designs.
- Implement sample conversion with saturation, correct sign extension,
  packed/unpacked 24-bit handling, endianness, and optional dither where it is
  part of the compatibility contract.
- Implement channel mapping, duplication, averaging, silence insertion, and
  route matrices with explicit clipping behavior.
- Implement sample-rate conversion with stated quality/latency profiles. Begin
  with a well-tested band-limited converter; do not use linear interpolation as
  the production-quality default.
- Implement `plug`/`plughw` negotiation so application and slave parameters are
  independently visible and latency accounting includes converters.
- Add `null`, `copy`, `route`, `file`, and `tee` only with documented libasound
  compatibility and bounded resource behavior.
- Test spectral quality, passband, alias rejection, clipping, DC behavior,
  deterministic output, latency, partial buffers, and long-run clock accounting.

Exit criteria: common `default` and `plughw` configurations work for application
formats/rates/channels not directly accepted by hardware, within published audio
quality and latency tolerances.

## 12. Phase 8: shared PCM plugins

- Specify `dmix`, `dsnoop`, and `dshare` IPC compatibility before coding. Decide
  whether the goal is interoperability with libasound clients, package-internal
  multi-client operation, or both.
- Implement shared-memory layout, semaphore/lock behavior, ownership cleanup,
  permissions, ring pointers, mixing arithmetic, saturation, client liveness,
  timestamps, and slow/failed client handling.
- Test native Go clients alongside libasound clients if interoperability is
  claimed. Include crash recovery, stale IPC objects, differing UIDs, hotplug,
  clock drift, XRUNs, and many concurrent readers/writers.
- Provide a safer package-local broker mode if exact System V IPC compatibility
  is infeasible, but expose it under a distinct name rather than calling it
  `dmix` or `dsnoop`.

Exit criteria: the documented concurrency mode survives multi-day stress tests
without corruption, deadlocks, unbounded latency, or leaked IPC resources.

## 13. External sound servers and extension boundary

- Detect configurations that terminate in PulseAudio, PipeWire, JACK, BlueALSA,
  or an unknown binary plugin and return a typed `UnsupportedPluginError` that
  names the plugin and resolution chain.
- Design an optional Go registration interface for externally implemented PCM
  nodes, while preventing configuration files from loading arbitrary code.
- Scope server-native submodules separately if common `default` configurations
  require them. Each must remain cgo-free and use the server's documented wire
  protocol; they are not part of the kernel ALSA implementation.
- Never fall back silently from a server-backed default device to direct
  hardware, because that bypasses desktop routing, policy, and mixing.

## 14. Public API design and stability

- Start under a pre-v1 module and publish design examples before freezing names.
- Separate requested parameters from negotiated parameters.
- Make context-aware operations first-class without forcing callers to manage
  nonblocking errno loops.
- Support both frame-oriented byte/sample buffers and low-level channel areas
  where needed; make frame counts unambiguous in every method.
- Define ownership and concurrency for every handle. State exactly which calls
  may run concurrently and how `Close` interacts with them.
- Use sentinel/typed errors for busy, XRUN, suspended, disconnected, malformed
  configuration, unsupported format/plugin/platform, permissions, and invalid
  state; retain the underlying `syscall.Errno`.
- Provide structured diagnostics or optional tracing for device resolution,
  negotiation, state transitions, recovery, and latency without logging from
  the library by default.
- Avoid global mutable configuration and hidden goroutines. Any background work
  must have explicit lifetime and shutdown semantics.

## 15. Testing strategy

### Unit and fuzz tests

- ABI bitmaps, masks, intervals, boundaries, ioctl payloads, and errno paths.
- Device-name and configuration parsing/evaluation, including malformed and
  adversarial input.
- PCM state/recovery logic through a scripted syscall layer.
- Conversion golden vectors and spectral/numerical property tests.
- Mixer grouping and TLV parsing.
- Fuzz all parsers, TLV decoders, name resolvers, negotiation code, and frame
  arithmetic with strict allocation and time limits.

### Differential tests

- Build small reference helpers using libasound only in a separate test/tooling
  context, never in the production module.
- Compare enumeration, parameter negotiation, resolved configurations, PCM
  output, control metadata/value operations, mixer behavior, and error cases.
- Normalize values that are inherently timing- or device-dependent and explain
  every accepted difference.

### Virtual and real hardware tests

- Use ALSA loopback and dummy modules where CI permissions allow.
- Maintain opt-in hardware suites for USB mono/stereo codecs, multichannel USB,
  HDA, Raspberry Pi/embedded codecs, and devices with constrained native rates.
- Exercise unplug/replug, suspend/resume, busy devices, permissions, XRUN storms,
  short periods, long captures, simultaneous clients, and clock drift.
- Run audio loopback checks for bit accuracy, channel order, latency, rate error,
  and signal quality.

### CI matrix

- `CGO_ENABLED=0` build and test on every commit.
- Native amd64 and arm64 tests, race detector, static analysis, formatting,
  vulnerability scan, fuzz smoke tests, and reproducible generation checks.
- Cross-build all supported architectures and run ABI/conformance tests under
  native or QEMU environments before release.
- Test against the minimum supported Go/kernel and current stable versions.

## 16. Performance and realtime requirements

- Benchmark syscall and mmap paths, conversion stages, mixer/control event
  handling, and end-to-end latency against comparable libasound configurations.
- Establish allocation budgets for steady-state PCM operations; target zero
  allocations after setup for common read/write paths.
- Use bounded buffers and overflow-safe frame/byte arithmetic everywhere.
- Do not promise hard realtime behavior from Go. Document scheduler and GC
  considerations, expose buffer/period controls, and measure tail latency.
- Prefer clear, verified synchronization over lock-free code. Use atomics only
  with documented memory-order reasoning relevant to shared PCM pointers.

## 17. Security and robustness review

- Treat ALSA configuration files, device metadata, TLVs, event streams, and
  kernel-returned lengths as untrusted input.
- Bound recursion, allocation, path expansion, graph size, channel counts, frame
  multiplication, and ioctl retry loops.
- Prevent descriptor-reuse races and unsafe-pointer lifetime violations.
- Do not execute configuration hooks, shell substitutions, or arbitrary dynamic
  plugins.
- Review device and IPC permissions; never broaden them automatically.
- Run fuzzing and static analysis over every unsafe or architecture-specific
  boundary. Keep `unsafe` confined to `internal/uapi` and narrowly reviewed I/O
  wrappers wherever possible.

## 18. Documentation and examples

- Package documentation explaining ALSA kernel interfaces versus libasound
  configuration/plugins.
- PCM capture/playback, duplex, poll/cancellation, enumeration, control events,
  boolean control, mixer mute/volume, and custom configuration examples.
- A compatibility table listing supported names/plugins and known differences.
- Troubleshooting for permissions, busy devices, desktop sound servers,
  negotiation errors, XRUNs, latency, and container device mapping.
- Architecture-support and kernel-version policies.
- Migration guide from common libasound C calls and existing Go cgo wrappers.
- Implement go-amixer, go-aplay, and go-arecord command line tools that perform
  the same functions as their ALSA equivalents, but using go-idioms.

## 19. Release gates

### v0.1: direct kernel foundation

- Generated UAPI for amd64/arm64.
- Hardware discovery, direct `hw` PCM S16_LE interleaved I/O, cancellation,
  XRUN recovery, and raw boolean control access.
- Unit, race, virtual-device, and at least two real-device test reports.

### v0.2: complete direct PCM and controls

- Broader formats/access modes, mmap, robust state operations, control types,
  TLVs, and events.
- Cross-architecture conformance and downstream modem integration.

### v0.3: configuration and mixer

- ALSA configuration parser/resolver, common `default` resolution, and simple
  mixer behavior with differential tests.

### v0.4: plug and routing

- Production-quality format/rate/channel conversion plus `plug`, `plughw`, and
  prioritized routing plugins.

### v0.5: shared devices

- Precisely documented `dmix`/`dsnoop`/`dshare` behavior and stress-tested
  concurrency, or an explicit decision to defer exact compatibility.

### v1.0

- Stable public API and compatibility contract.
- amd64 and arm64 release gates green on the supported kernel range.
- No production cgo dependency, complete shutdown/cancellation semantics, no
  known descriptor/IPC leaks, and downstream validation in Vulpes.
- Every unsupported configuration/plugin returns an actionable typed error.

## 20. Downstream validation with Vulpes

- Add go-alsa behind Vulpes' existing audio interfaces without initially
  removing its cgo backend, allowing identical hardware A/B tests.
- Compare negotiated parameters, capture quality, transmit latency, cancellation,
  XRUN counters/recovery, CPU use, and long-run stability.
- Port Vulpes' generic ALSA boolean PTT control to the raw control API.
- Port mixer mute PTT only after simple-mixer semantics pass differential tests.
- Test the explicit hardware names used by radio interfaces, plus `default` and
  `plughw` once their graph implementations are ready.
- Remove the Vulpes cgo path only after feature parity is demonstrated on amd64
  and arm64 hardware and static `CGO_ENABLED=0` builds pass operational tests.

## 21. Project management rules

- Maintain the compatibility matrix and phase exit criteria in the repository;
  do not mark features complete based solely on compilation.
- Keep kernel-UAPI work, PCM behavior, configuration semantics, conversion DSP,
  mixer policy, and shared-device IPC as reviewable changes with focused tests.
- Require hardware evidence for hardware-facing claims and differential evidence
  for libasound-compatibility claims.
- Record intentional incompatibilities in release notes and the compatibility
  table before merging them.
- Prefer a smaller truthful release over silently accepting a device name whose
  semantics the package does not implement.
