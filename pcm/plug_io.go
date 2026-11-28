package pcm

import (
	"context"
	"errors"
	"fmt"

	alsa "github.com/sparques/go-alsa"
)

// WriteInterleavedBytes converts one complete application-domain playback
// chunk and writes all produced hardware frames. Converted and scratch are
// caller-owned buffers sized with PendingFrames and ScratchSamples. Once
// conversion succeeds, all application frames have been consumed; they are
// returned even if a later hardware write fails. This method allocates no
// memory and is not safe for concurrent use with other converter operations.
func (c *PlugConverter) WriteInterleavedBytes(ctx context.Context, h *Handle, converted, application []byte, scratch []int32) (int, error) {
	if c == nil || c.StreamConverter == nil {
		return 0, errors.New("alsa pcm: uninitialized plug converter")
	}
	if h == nil {
		return 0, errors.New("alsa pcm: plug playback requires a handle")
	}
	if c.stream != alsa.Playback || h.stream != streamPlayback {
		return 0, ErrDirection
	}
	applicationFrameBytes, err := conversionFrameBytes(c.application)
	if err != nil {
		return 0, err
	}
	if len(application)%applicationFrameBytes != 0 {
		return 0, errors.New("alsa pcm: plug playback application buffer contains a partial frame")
	}
	applicationFrames := len(application) / applicationFrameBytes
	hardwareFrames, err := c.Process(converted, application, scratch)
	if err != nil {
		return 0, err
	}
	hardwareFrameBytes, err := conversionFrameBytes(c.hardware)
	if err != nil {
		return 0, err
	}
	converted = converted[:hardwareFrames*hardwareFrameBytes]
	for writtenFrames := 0; writtenFrames < hardwareFrames; {
		written, writeErr := h.WriteInterleavedBytes(ctx, converted[writtenFrames*hardwareFrameBytes:])
		writtenFrames += written
		if writeErr != nil {
			return applicationFrames, writeErr
		}
		if written == 0 {
			return applicationFrames, errors.New("alsa pcm: plug playback hardware write made no progress")
		}
	}
	return applicationFrames, nil
}

// ReadInterleavedBytes captures into hardware, converts only the frames
// actually read, and returns application-domain frames in application. The
// hardware, application, and scratch buffers are caller-owned. Application
// may be sized for the maximum output predicted from the hardware buffer.
// This method allocates no memory and is not safe for concurrent use with
// other converter operations.
func (c *PlugConverter) ReadInterleavedBytes(ctx context.Context, h *Handle, application, hardware []byte, scratch []int32) (int, error) {
	if c == nil || c.StreamConverter == nil {
		return 0, errors.New("alsa pcm: uninitialized plug converter")
	}
	if h == nil {
		return 0, errors.New("alsa pcm: plug capture requires a handle")
	}
	if c.stream != alsa.Capture || h.stream != streamCapture {
		return 0, ErrDirection
	}
	hardwareFrameBytes, err := conversionFrameBytes(c.hardware)
	if err != nil {
		return 0, err
	}
	if len(hardware)%hardwareFrameBytes != 0 {
		return 0, errors.New("alsa pcm: plug capture hardware buffer contains a partial frame")
	}
	readFrames, err := h.ReadInterleavedBytes(ctx, hardware)
	if err != nil {
		return 0, err
	}
	return c.Process(application, hardware[:readFrames*hardwareFrameBytes], scratch)
}

func conversionFrameBytes(parameters ConversionParams) (int, error) {
	width, ok := parameters.Format.PhysicalBytes()
	if !ok || parameters.Channels <= 0 || width > maximumInt/parameters.Channels {
		return 0, fmt.Errorf("alsa pcm: invalid conversion frame representation")
	}
	return width * parameters.Channels, nil
}
