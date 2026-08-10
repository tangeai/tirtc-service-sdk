package main

import (
	"bytes"
	"image/jpeg"
	"testing"
)

func TestSinePCM(t *testing.T) {
	frame := sinePCM(0)
	if len(frame) != 320 {
		t.Fatalf("PCM bytes = %d, want 320", len(frame))
	}
	if bytes.Equal(frame, make([]byte, len(frame))) {
		t.Fatal("PCM frame is silent")
	}
}

func TestSyntheticJPEG(t *testing.T) {
	config, err := jpeg.DecodeConfig(bytes.NewReader(syntheticJPEG(3)))
	if err != nil {
		t.Fatalf("decode JPEG: %v", err)
	}
	if config.Width != 320 || config.Height != 240 {
		t.Fatalf("JPEG size = %dx%d", config.Width, config.Height)
	}
}

func TestBusinessMediaStreamIDsAreDistinctAndValid(t *testing.T) {
	if businessAudioStreamID == businessVideoStreamID {
		t.Fatal("audio and video stream IDs must be distinct")
	}
	if businessAudioStreamID > 15 || businessVideoStreamID > 15 {
		t.Fatal("business stream IDs must be in the range 0..15")
	}
}
