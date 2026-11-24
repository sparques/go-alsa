//go:build linux && (amd64 || arm64)

package uapi

import "unsafe"

const (
	PCMProtocolVersion = 2<<16 | 0<<8 | 18
	pcmIoctlType       = uintptr('A')

	PCMStreamPlayback = int32(0)
	PCMStreamCapture  = int32(1)

	PCMHWParamAccess        = 0
	PCMHWParamFormat        = 1
	PCMHWParamSubformat     = 2
	PCMHWParamFirstMask     = PCMHWParamAccess
	PCMHWParamLastMask      = PCMHWParamSubformat
	PCMHWParamSampleBits    = 8
	PCMHWParamFrameBits     = 9
	PCMHWParamChannels      = 10
	PCMHWParamRate          = 11
	PCMHWParamPeriodTime    = 12
	PCMHWParamPeriodSize    = 13
	PCMHWParamPeriodBytes   = 14
	PCMHWParamPeriods       = 15
	PCMHWParamBufferTime    = 16
	PCMHWParamBufferSize    = 17
	PCMHWParamBufferBytes   = 18
	PCMHWParamTickTime      = 19
	PCMHWParamFirstInterval = PCMHWParamSampleBits
	PCMHWParamLastInterval  = PCMHWParamTickTime

	PCMSubformatStandard = uint32(0)
	PCMIntervalOpenMin   = uint32(1 << 0)
	PCMIntervalOpenMax   = uint32(1 << 1)
	PCMIntervalInteger   = uint32(1 << 2)
	PCMIntervalEmpty     = uint32(1 << 3)
)

// PCMInfo matches struct snd_pcm_info from Linux UAPI sound/asound.h.
type PCMInfo struct {
	Device              uint32
	Subdevice           uint32
	Stream              int32
	Card                int32
	ID                  [64]byte
	Name                [80]byte
	Subname             [32]byte
	DeviceClass         int32
	DeviceSubclass      int32
	SubdevicesCount     uint32
	SubdevicesAvailable uint32
	Pad                 [16]byte
	Reserved            [64]byte
}

type PCMMask struct {
	Bits [8]uint32
}

// PCMInterval represents snd_interval. Flags contains the four one-bit C
// bitfields in declaration order: openmin, openmax, integer, and empty.
type PCMInterval struct {
	Min   uint32
	Max   uint32
	Flags uint32
}

// PCMHWParams matches struct snd_pcm_hw_params on supported 64-bit Linux
// architectures.
type PCMHWParams struct {
	Flags             uint32
	Masks             [3]PCMMask
	ReservedMasks     [5]PCMMask
	Intervals         [12]PCMInterval
	ReservedIntervals [9]PCMInterval
	RequestMask       uint32
	ChangedMask       uint32
	Info              uint32
	MSBits            uint32
	RateNumerator     uint32
	RateDenominator   uint32
	FIFOSize          uint64
	SyncID            [16]byte
	Reserved          [48]byte
}

// PCMSWParams matches struct snd_pcm_sw_params on supported 64-bit Linux
// architectures.
type PCMSWParams struct {
	TimestampMode    int32
	PeriodStep       uint32
	SleepMinimum     uint32
	Pad              uint32
	AvailableMinimum uint64
	TransferAlign    uint64
	StartThreshold   uint64
	StopThreshold    uint64
	SilenceThreshold uint64
	SilenceSize      uint64
	Boundary         uint64
	Protocol         uint32
	TimestampType    uint32
	Reserved         [56]byte
}

// PCMXferI matches struct snd_xferi for interleaved frame transfers.
type PCMXferI struct {
	Result int64
	Buffer uintptr
	Frames uint64
}

// PCMXferN matches struct snd_xfern for non-interleaved frame transfers.
type PCMXferN struct {
	Result  int64
	Buffers uintptr
	Frames  uint64
}

type Timespec struct {
	Seconds     int64
	Nanoseconds int64
}

// PCMStatus matches struct snd_pcm_status on supported 64-bit Linux
// architectures.
type PCMStatus struct {
	State                  int32
	Pad1                   [4]byte
	TriggerTimestamp       Timespec
	Timestamp              Timespec
	ApplicationPointer     uint64
	HardwarePointer        uint64
	Delay                  int64
	Available              uint64
	MaximumAvailable       uint64
	Overrange              uint64
	SuspendedState         int32
	AudioTimestampData     uint32
	AudioTimestamp         Timespec
	DriverTimestamp        Timespec
	AudioTimestampAccuracy uint32
	Reserved               [20]byte
}

const (
	PCMIoctlProtocolVersion     = uintptr(0x80044100)
	PCMIoctlInfo                = uintptr(0x81204101)
	PCMIoctlHWRefine            = uintptr(0xc2604110)
	PCMIoctlHWParams            = uintptr(0xc2604111)
	PCMIoctlHWFree              = uintptr(0x00004112)
	PCMIoctlSWParams            = uintptr(0xc0884113)
	PCMIoctlStatus              = uintptr(0x80984120)
	PCMIoctlDelay               = uintptr(0x80084121)
	PCMIoctlHWSync              = uintptr(0x00004122)
	PCMIoctlPrepare             = uintptr(0x00004140)
	PCMIoctlReset               = uintptr(0x00004141)
	PCMIoctlStart               = uintptr(0x00004142)
	PCMIoctlDrop                = uintptr(0x00004143)
	PCMIoctlDrain               = uintptr(0x00004144)
	PCMIoctlPause               = uintptr(0x40044145)
	PCMIoctlRewind              = uintptr(0x40084146)
	PCMIoctlResume              = uintptr(0x00004147)
	PCMIoctlForward             = uintptr(0x40084149)
	PCMIoctlWriteInterleaved    = uintptr(0x40184150)
	PCMIoctlReadInterleaved     = uintptr(0x80184151)
	PCMIoctlWriteNonInterleaved = uintptr(0x40184152)
	PCMIoctlReadNonInterleaved  = uintptr(0x80184153)
	PCMIoctlLink                = uintptr(0x40044160)
	PCMIoctlUnlink              = uintptr(0x00004161)

	ControlIoctlPCMNextDevice = uintptr(0x80045530)
	ControlIoctlPCMInfo       = uintptr(0xc1205531)
)

var (
	_ [288 - unsafe.Sizeof(PCMInfo{})]byte
	_ [unsafe.Sizeof(PCMInfo{}) - 288]byte
	_ [608 - unsafe.Sizeof(PCMHWParams{})]byte
	_ [unsafe.Sizeof(PCMHWParams{}) - 608]byte
	_ [136 - unsafe.Sizeof(PCMSWParams{})]byte
	_ [unsafe.Sizeof(PCMSWParams{}) - 136]byte
	_ [24 - unsafe.Sizeof(PCMXferI{})]byte
	_ [unsafe.Sizeof(PCMXferI{}) - 24]byte
	_ [24 - unsafe.Sizeof(PCMXferN{})]byte
	_ [unsafe.Sizeof(PCMXferN{}) - 24]byte
	_ [152 - unsafe.Sizeof(PCMStatus{})]byte
	_ [unsafe.Sizeof(PCMStatus{}) - 152]byte
)
