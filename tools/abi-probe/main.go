//go:build linux && (amd64 || arm64)

// Command abi-probe verifies checked-in Go ALSA layouts against the Linux UAPI
// headers installed on the maintainer's machine. Production builds do not use
// this command and remain independent of cgo and C headers.
package main

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"github.com/sparques/go-alsa/internal/uapi"
)

//go:embed source/probe.c
var probeSource []byte

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "abi-probe:", err)
		os.Exit(1)
	}
}

func run() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("requires Linux, got %s", runtime.GOOS)
	}
	directory, err := os.MkdirTemp("", "go-alsa-abi-probe-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)

	source := filepath.Join(directory, "probe.c")
	binary := filepath.Join(directory, "probe")
	if err := os.WriteFile(source, probeSource, 0o600); err != nil {
		return err
	}
	compiler := os.Getenv("CC")
	if compiler == "" {
		compiler = "cc"
	}
	command := exec.Command(compiler, "-std=c11", "-Wall", "-Wextra", "-Werror", source, "-o", binary)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("compile C probe with %s: %w\n%s", compiler, err, output)
	}
	output, err := exec.Command(binary).Output()
	if err != nil {
		return fmt.Errorf("run C probe: %w", err)
	}
	cValues, err := parseValues(output)
	if err != nil {
		return err
	}

	goValues := map[string]uint64{
		"control_protocol":             uapi.ControlProtocolVersion,
		"control_card_info_size":       uint64(unsafe.Sizeof(uapi.ControlCardInfo{})),
		"control_components_offset":    uint64(unsafe.Offsetof(uapi.ControlCardInfo{}.Components)),
		"control_pversion_ioctl":       uint64(uapi.ControlIoctlProtocolVersion),
		"control_card_info_ioctl":      uint64(uapi.ControlIoctlCardInfo),
		"control_element_id_size":      uint64(unsafe.Sizeof(uapi.ControlElementID{})),
		"control_element_list_size":    uint64(unsafe.Sizeof(uapi.ControlElementList{})),
		"control_list_ids_offset":      uint64(unsafe.Offsetof(uapi.ControlElementList{}.IDs)),
		"control_element_info_size":    uint64(unsafe.Sizeof(uapi.ControlElementInfo{})),
		"control_info_value_offset":    uint64(unsafe.Offsetof(uapi.ControlElementInfo{}.Value)),
		"control_element_list_ioctl":   uint64(uapi.ControlIoctlElementList),
		"control_element_info_ioctl":   uint64(uapi.ControlIoctlElementInfo),
		"control_element_value_size":   uint64(unsafe.Sizeof(uapi.ControlElementValue{})),
		"control_value_data_offset":    uint64(unsafe.Offsetof(uapi.ControlElementValue{}.Value)),
		"control_element_read_ioctl":   uint64(uapi.ControlIoctlElementRead),
		"control_element_write_ioctl":  uint64(uapi.ControlIoctlElementWrite),
		"control_element_lock_ioctl":   uint64(uapi.ControlIoctlElementLock),
		"control_element_unlock_ioctl": uint64(uapi.ControlIoctlElementUnlock),
		"control_event_size":           uint64(unsafe.Sizeof(uapi.ControlEvent{})),
		"control_event_id_offset":      uint64(unsafe.Offsetof(uapi.ControlEvent{}.ID)),
		"control_subscribe_ioctl":      uint64(uapi.ControlIoctlSubscribeEvents),
		"control_tlv_header_size":      uint64(8),
		"control_tlv_read_ioctl":       uint64(uapi.ControlIoctlTLVRead),
		"control_tlv_write_ioctl":      uint64(uapi.ControlIoctlTLVWrite),
		"control_tlv_command_ioctl":    uint64(uapi.ControlIoctlTLVCommand),
		"pcm_protocol":                 uapi.PCMProtocolVersion,
		"pcm_pversion_ioctl":           uint64(uapi.PCMIoctlProtocolVersion),
		"pcm_info_ioctl":               uint64(uapi.PCMIoctlInfo),
		"pcm_hw_params_size":           uint64(unsafe.Sizeof(uapi.PCMHWParams{})),
		"pcm_hw_intervals_offset":      uint64(unsafe.Offsetof(uapi.PCMHWParams{}.Intervals)),
		"pcm_hw_fifo_offset":           uint64(unsafe.Offsetof(uapi.PCMHWParams{}.FIFOSize)),
		"pcm_hw_refine_ioctl":          uint64(uapi.PCMIoctlHWRefine),
		"pcm_hw_params_ioctl":          uint64(uapi.PCMIoctlHWParams),
		"pcm_hw_free_ioctl":            uint64(uapi.PCMIoctlHWFree),
		"pcm_sw_params_size":           uint64(unsafe.Sizeof(uapi.PCMSWParams{})),
		"pcm_sw_avail_offset":          uint64(unsafe.Offsetof(uapi.PCMSWParams{}.AvailableMinimum)),
		"pcm_sw_protocol_offset":       uint64(unsafe.Offsetof(uapi.PCMSWParams{}.Protocol)),
		"pcm_sw_params_ioctl":          uint64(uapi.PCMIoctlSWParams),
		"pcm_prepare_ioctl":            uint64(uapi.PCMIoctlPrepare),
		"pcm_start_ioctl":              uint64(uapi.PCMIoctlStart),
		"pcm_drop_ioctl":               uint64(uapi.PCMIoctlDrop),
		"pcm_drain_ioctl":              uint64(uapi.PCMIoctlDrain),
		"pcm_xferi_size":               uint64(unsafe.Sizeof(uapi.PCMXferI{})),
		"pcm_writei_ioctl":             uint64(uapi.PCMIoctlWriteInterleaved),
		"pcm_readi_ioctl":              uint64(uapi.PCMIoctlReadInterleaved),
		"pcm_xfern_size":               uint64(unsafe.Sizeof(uapi.PCMXferN{})),
		"pcm_writen_ioctl":             uint64(uapi.PCMIoctlWriteNonInterleaved),
		"pcm_readn_ioctl":              uint64(uapi.PCMIoctlReadNonInterleaved),
		"pcm_status_size":              uint64(unsafe.Sizeof(uapi.PCMStatus{})),
		"pcm_status_appl_offset":       uint64(unsafe.Offsetof(uapi.PCMStatus{}.ApplicationPointer)),
		"pcm_status_audio_offset":      uint64(unsafe.Offsetof(uapi.PCMStatus{}.AudioTimestamp)),
		"pcm_status_ioctl":             uint64(uapi.PCMIoctlStatus),
		"pcm_delay_ioctl":              uint64(uapi.PCMIoctlDelay),
		"pcm_hwsync_ioctl":             uint64(uapi.PCMIoctlHWSync),
		"pcm_reset_ioctl":              uint64(uapi.PCMIoctlReset),
		"pcm_pause_ioctl":              uint64(uapi.PCMIoctlPause),
		"pcm_rewind_ioctl":             uint64(uapi.PCMIoctlRewind),
		"pcm_resume_ioctl":             uint64(uapi.PCMIoctlResume),
		"pcm_forward_ioctl":            uint64(uapi.PCMIoctlForward),
		"pcm_info_size":                uint64(unsafe.Sizeof(uapi.PCMInfo{})),
		"pcm_subdevices_offset":        uint64(unsafe.Offsetof(uapi.PCMInfo{}.SubdevicesCount)),
		"control_pcm_next_ioctl":       uint64(uapi.ControlIoctlPCMNextDevice),
		"control_pcm_info_ioctl":       uint64(uapi.ControlIoctlPCMInfo),
	}

	failed := false
	for name, goValue := range goValues {
		cValue, exists := cValues[name]
		if !exists {
			return fmt.Errorf("C probe omitted %q", name)
		}
		status := "ok"
		if cValue != goValue {
			status = "MISMATCH"
			failed = true
		}
		fmt.Printf("%-30s C=%#x Go=%#x %s\n", name, cValue, goValue, status)
	}
	if failed {
		return fmt.Errorf("Linux UAPI and checked-in Go definitions differ")
	}
	return nil
}

func parseValues(output []byte) (map[string]uint64, error) {
	values := make(map[string]uint64)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		name, text, ok := strings.Cut(scanner.Text(), "=")
		if !ok || name == "" || text == "" {
			return nil, fmt.Errorf("malformed C probe output %q", scanner.Text())
		}
		value, err := strconv.ParseUint(text, 0, 64)
		if err != nil {
			return nil, fmt.Errorf("parse C probe value %q: %w", scanner.Text(), err)
		}
		values[name] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}
