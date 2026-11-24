// alsa-pcm-go streams headerless PCM between a direct ALSA hardware device and
// standard input/output. It is intentionally small: besides being useful for
// smoke tests, it demonstrates the complete native PCM setup sequence.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	alsa "github.com/sparques/go-alsa"
	"github.com/sparques/go-alsa/pcm"
)

type options struct {
	device    string
	capture   bool
	format    string
	rate      int
	channels  int
	period    int
	buffer    int
	duration  time.Duration
	exact     bool
	showSetup bool
}

func main() {
	var opts options
	flag.StringVar(&opts.device, "device", "hw:0,0", "direct ALSA hardware device")
	flag.BoolVar(&opts.capture, "capture", false, "capture to stdout instead of playing stdin")
	flag.StringVar(&opts.format, "format", "s16le", "sample format: s8, u8, s16le, s24le, s24_3le, or s32le")
	flag.IntVar(&opts.rate, "rate", 48000, "sample rate")
	flag.IntVar(&opts.channels, "channels", 2, "channel count")
	flag.IntVar(&opts.period, "period", 480, "period size in frames")
	flag.IntVar(&opts.buffer, "buffer", 4800, "buffer size in frames")
	flag.DurationVar(&opts.duration, "duration", 0, "stop after this duration (zero means until EOF or signal)")
	flag.BoolVar(&opts.exact, "exact", false, "require exact hardware parameters")
	flag.BoolVar(&opts.showSetup, "show-setup", true, "print negotiated parameters to stderr")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if opts.duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.duration)
		defer cancel()
	}
	if err := run(ctx, opts, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, opts options, input io.Reader, output io.Writer, diagnostics io.Writer) error {
	format, err := parseFormat(opts.format)
	if err != nil {
		return err
	}
	width, _ := format.PhysicalBytes()
	config := pcm.HardwareConfig{
		Access: pcm.RWInterleaved, Format: format, Channels: opts.channels,
		Rate: opts.rate, PeriodFrames: opts.period, BufferFrames: opts.buffer,
	}
	stream := alsa.Playback
	if opts.capture {
		stream = alsa.Capture
	}
	handle, err := pcm.OpenHardware(opts.device, stream)
	if err != nil {
		return err
	}
	defer handle.Close()

	var setup pcm.HardwareSetup
	if opts.exact {
		setup, err = handle.ConfigureHardware(config)
	} else {
		setup, err = handle.ConfigureHardwareNear(config)
	}
	if err != nil {
		return err
	}
	if opts.showSetup {
		fmt.Fprintf(diagnostics, "%s %s: %s, %d channels, %d Hz, period %d, buffer %d frames\n",
			opts.device, stream, setup.Format, setup.Channels, setup.Rate, setup.PeriodFrames, setup.BufferFrames)
	}
	if _, err = handle.ConfigureSoftwareDefaults(); err != nil {
		return err
	}
	if err = handle.Prepare(); err != nil {
		return err
	}
	frameBytes := setup.Channels * width
	buffer := make([]byte, setup.PeriodFrames*frameBytes)
	if opts.capture {
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
			if writeErr := writeAll(output, buffer[:frames*frameBytes]); writeErr != nil {
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

func writeAll(writer io.Writer, buffer []byte) error {
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

func parseFormat(name string) (pcm.Format, error) {
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
