package main

import (
	"os"
	"testing"

	"github.com/rwcarlsen/goexif/exif"
)

func TestExifGPS(t *testing.T) {
	f, err := os.Open("testdata/test.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	x, err := exif.Decode(f)
	if err != nil {
		t.Fatal(err)
	}

	lat, lng, err := x.LatLong()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("lat=%f lng=%f", lat, lng)
}
