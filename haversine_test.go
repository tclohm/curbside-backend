package main

import (
	"math"
	"testing"
)

func TestHaversineDistanceMeters(t *testing.T) {
	t.Run("same point is zero distance", func(t *testing.T) {
		d := haversineDistanceMeters(33.7952, -118.3088, 33.7952, -118.3088)
		if d != 0 {
			t.Errorf("distance = %f, want 0", d)
		}
	})

	t.Run("one degree of longitude at equator is about 111.19km", func(t *testing.T) {
		d := haversineDistanceMeters(0, 0, 0, 1)
		want := 111195.0
		tolerance := 100.0 // meters 
		if math.Abs(d - want) > tolerance {
			t.Errorf("distance = %f, want ~%f (+/- %f)", d, want, tolerance)
		}
	})

	t.Run("small offset within threshold", func(t *testing.T) {
		// -0.001 degrees of latitude is ~111m the 150m threshold 
		d := haversineDistanceMeters(33.7952, -118.3088, 33.7962, -118.3088)
		if d > gpsMismatchThresholdMeters {
			t.Errorf("distance = %f, want under threshold %f", d, gpsMismatchThresholdMeters)
		}
	})

	t.Run("large offset exceeds threshold", func(t *testing.T) {
		// About 1km away in latitude - should clearly fail a 150m threshold
		d := haversineDistanceMeters(33.7952, -118.3088, 33.8047, -118.3088)
		if d < gpsMismatchThresholdMeters {
			t.Errorf("distance = %f, want over threshold %f", d, gpsMismatchThresholdMeters)
		}
	})
}
