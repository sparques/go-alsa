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
	config := pcmtool.Config{Capture: true}
	var duration time.Duration
	flag.StringVar(&config.Device, "device", "hw:0,0", "direct ALSA hardware device")
	flag.StringVar(&config.Format, "format", "s16le", "sample format")
	flag.IntVar(&config.Rate, "rate", 48000, "sample rate")
	flag.IntVar(&config.Channels, "channels", 2, "channel count")
	flag.IntVar(&config.Period, "period", 480, "period frames")
	flag.IntVar(&config.Buffer, "buffer", 4800, "buffer frames")
	flag.DurationVar(&duration, "duration", 0, "stop after this duration")
	flag.BoolVar(&config.Exact, "exact", false, "require exact parameters")
	flag.BoolVar(&config.ShowSetup, "show-setup", true, "print negotiated parameters")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, duration)
		defer cancel()
	}
	if err := pcmtool.Run(ctx, config, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
