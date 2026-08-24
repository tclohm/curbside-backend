package main

import (
	"os"
	"testing"
)

func TestIsJPEG(t *testing.T) {
	t.Run("real JPEG", func(t *testing.T) {
		data, err := os.ReadFile("testdata/test_converted.jpg")
		if err != nil {
			t.Fatal(err)
		}
		if !isJPEG(data) {
			t.Error("isJPEG(real JPEG) = false, want true")
		}
	})

	t.Run("HEIC file wearing a .jpg extension", func(t *testing.T) {
		data, err := os.ReadFile("testdata/test.jpg")
		if err != nil {
			t.Fatal(err)
		}
		if isJPEG(data) {
			t.Error("isJPEG(HEIC) = true, want false")
		}
	})

	t.Run("empty data", func(t *testing.T) {
		if isJPEG([]byte{}) {
			t.Error("isJPEG(empty) = true, want false")
		}
	})
}
