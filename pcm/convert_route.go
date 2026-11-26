package pcm

import "fmt"

// ConvertRouteScratchSamples returns the number of int32 scratch samples
// required to convert and route frames. The scratch holds decoded input and
// routed output concurrently.
func ConvertRouteScratchSamples(frames, sourceChannels, destinationChannels int) (int, error) {
	if frames < 0 {
		return 0, fmt.Errorf("alsa pcm: negative conversion frame count")
	}
	if sourceChannels <= 0 || sourceChannels > maximumRouteChannels || destinationChannels <= 0 || destinationChannels > maximumRouteChannels {
		return 0, fmt.Errorf("alsa pcm: conversion channels must be between 1 and %d", maximumRouteChannels)
	}
	channels := sourceChannels + destinationChannels
	if frames > maximumInt/channels {
		return 0, fmt.Errorf("alsa pcm: conversion scratch size overflows int")
	}
	return frames * channels, nil
}

// ConvertRouteInterleaved decodes, channel-routes, and encodes complete
// interleaved frames using caller-provided scratch storage. It performs no
// allocation. Source, destination, and scratch must not overlap.
func ConvertRouteInterleaved(destination []byte, destinationFormat Format, destinationChannels int, source []byte, sourceFormat Format, sourceChannels int, matrix []int32, scratch []int32) (int, error) {
	sourceLayout, err := integerLayout(sourceFormat)
	if err != nil {
		return 0, err
	}
	destinationLayout, err := integerLayout(destinationFormat)
	if err != nil {
		return 0, err
	}
	if sourceChannels <= 0 || sourceChannels > maximumRouteChannels || destinationChannels <= 0 || destinationChannels > maximumRouteChannels {
		return 0, fmt.Errorf("alsa pcm: conversion channels must be between 1 and %d", maximumRouteChannels)
	}
	frameBytes := sourceLayout.bytes * sourceChannels
	if len(source)%frameBytes != 0 {
		return 0, fmt.Errorf("alsa pcm: conversion source contains a partial frame")
	}
	frames := len(source) / frameBytes
	requiredScratch, err := ConvertRouteScratchSamples(frames, sourceChannels, destinationChannels)
	if err != nil {
		return 0, err
	}
	if len(scratch) < requiredScratch {
		return 0, fmt.Errorf("alsa pcm: conversion scratch holds %d samples, need %d", len(scratch), requiredScratch)
	}
	destinationSamples := frames * destinationChannels
	if destinationSamples > maximumInt/destinationLayout.bytes {
		return 0, fmt.Errorf("alsa pcm: conversion destination size overflows int")
	}
	neededBytes := destinationSamples * destinationLayout.bytes
	if len(destination) < neededBytes {
		return 0, fmt.Errorf("alsa pcm: conversion destination holds %d bytes, need %d", len(destination), neededBytes)
	}
	sourceSamples := frames * sourceChannels
	decoded := scratch[:sourceSamples]
	routed := scratch[sourceSamples:requiredScratch]
	if _, err := DecodeSamples(decoded, source, sourceFormat); err != nil {
		return 0, err
	}
	if _, err := RouteInterleaved(routed, destinationChannels, decoded, sourceChannels, matrix); err != nil {
		return 0, err
	}
	if _, err := EncodeSamples(destination[:neededBytes], routed, destinationFormat); err != nil {
		return 0, err
	}
	return frames, nil
}

const maximumInt = int(^uint(0) >> 1)
