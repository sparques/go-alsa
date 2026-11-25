// Package pcmtool implements raw PCM streaming shared by diagnostic commands.
package pcmtool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	alsa "github.com/sparques/go-alsa"
	"github.com/sparques/go-alsa/pcm"
)

type Config struct {
	Device                         string
	Capture                        bool
	Format                         string
	Rate, Channels, Period, Buffer int
	Exact, ShowSetup               bool
}

func Run(ctx context.Context, config Config, input io.Reader, output, diagnostics io.Writer) error {
	format, err := ParseFormat(config.Format)
	if err != nil {
		return err
	}
	width, _ := format.PhysicalBytes()
	hardware := pcm.HardwareConfig{Access: pcm.RWInterleaved, Format: format, Channels: config.Channels, Rate: config.Rate, PeriodFrames: config.Period, BufferFrames: config.Buffer}
	stream := alsa.Playback
	if config.Capture {
		stream = alsa.Capture
	}
	handle, err := pcm.OpenHardware(config.Device, stream)
	if err != nil {
		return err
	}
	defer handle.Close()
	var setup pcm.HardwareSetup
	if config.Exact {
		setup, err = handle.ConfigureHardware(hardware)
	} else {
		setup, err = handle.ConfigureHardwareNear(hardware)
	}
	if err != nil {
		return err
	}
	if config.ShowSetup {
		fmt.Fprintf(diagnostics, "%s %s: %s, %d channels, %d Hz, period %d, buffer %d frames\n", config.Device, stream, setup.Format, setup.Channels, setup.Rate, setup.PeriodFrames, setup.BufferFrames)
	}
	if _, err = handle.ConfigureSoftwareDefaults(); err != nil {
		return err
	}
	if err = handle.Prepare(); err != nil {
		return err
	}
	frameBytes := setup.Channels * width
	buffer := make([]byte, setup.PeriodFrames*frameBytes)
	if config.Capture {
		err = capture(ctx, handle, output, buffer, frameBytes)
	} else {
		err = playback(ctx, handle, input, buffer, frameBytes)
	}
	if err != nil {
		_ = handle.Drop()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
	}
	return err
}

func capture(ctx context.Context, handle *pcm.Handle, output io.Writer, buffer []byte, frameBytes int) error {
	if err := handle.Start(); err != nil {
		return err
	}
	for {
		frames, err := handle.ReadInterleavedBytes(ctx, buffer)
		if frames > 0 {
			if writeErr := WriteAll(output, buffer[:frames*frameBytes]); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			return err
		}
	}
}

func playback(ctx context.Context, handle *pcm.Handle, input io.Reader, buffer []byte, frameBytes int) error {
	for {
		count, readErr := io.ReadFull(input, buffer)
		if count%frameBytes != 0 {
			return fmt.Errorf("input ends with %d bytes outside a complete %d-byte frame", count%frameBytes, frameBytes)
		}
		for offset := 0; offset < count; {
			frames, err := handle.WriteInterleavedBytes(ctx, buffer[offset:count])
			offset += frames * frameBytes
			if err != nil {
				return err
			}
			if frames == 0 {
				return errors.New("ALSA playback made no progress")
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
				return readErr
			}
			return handle.Drain(ctx)
		}
	}
}

func WriteAll(writer io.Writer, buffer []byte) error {
	for len(buffer) > 0 {
		count, err := writer.Write(buffer)
		if err != nil {
			return err
		}
		if count == 0 {
			return io.ErrShortWrite
		}
		buffer = buffer[count:]
	}
	return nil
}

func ParseFormat(name string) (pcm.Format, error) {
	switch strings.ToLower(name) {
	case "s8":
		return pcm.S8, nil
	case "u8":
		return pcm.U8, nil
	case "s16le", "s16_le":
		return pcm.S16LE, nil
	case "s24le", "s24_le":
		return pcm.S24LE, nil
	case "s24_3le", "s24_3_le":
		return pcm.S24_3LE, nil
	case "s32le", "s32_le":
		return pcm.S32LE, nil
	default:
		return 0, fmt.Errorf("unsupported sample format %q", name)
	}
}
