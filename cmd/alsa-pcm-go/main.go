package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sparques/go-alsa/internal/pcmtool"
)

func main() {
	config := pcmtool.Config{}
	var duration time.Duration
	bindFlags(&config, &duration, false)
	flag.BoolVar(&config.Capture, "capture", false, "capture to stdout instead of playing stdin")
	flag.Parse()
	ctx, cancel := commandContext(duration)
	defer cancel()
	if err := pcmtool.Run(ctx, config, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func bindFlags(config *pcmtool.Config, duration *time.Duration, capture bool) {
	config.Capture = capture
	flag.StringVar(&config.Device, "device", "hw:0,0", "direct ALSA hardware device")
	flag.StringVar(&config.Format, "format", "s16le", "sample format")
	flag.IntVar(&config.Rate, "rate", 48000, "sample rate")
	flag.IntVar(&config.Channels, "channels", 2, "channel count")
	flag.IntVar(&config.Period, "period", 480, "period size in frames")
	flag.IntVar(&config.Buffer, "buffer", 4800, "buffer size in frames")
	flag.DurationVar(duration, "duration", 0, "stop after this duration")
	flag.BoolVar(&config.Exact, "exact", false, "require exact hardware parameters")
	flag.BoolVar(&config.ShowSetup, "show-setup", true, "print negotiated parameters")
}

func commandContext(duration time.Duration) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if duration <= 0 {
		return ctx, stop
	}
	timed, cancel := context.WithTimeout(ctx, duration)
	return timed, func() { cancel(); stop() }
}
