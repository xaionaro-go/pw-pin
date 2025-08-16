package sampleconfig

import (
	"github.com/xaionaro-go/pw-pin/pkg/pwpin"
)

var Get = func() pwpin.Config {
	appSelector := pwpin.Constraints{
		{Property: "media.name", Values: []string{" - mpv"}, Op: pwpin.ConstraintOpContains},
	}
	sinkSelector := pwpin.Constraints{
		{
			Property: "node.name",
			Values:   []string{"alsa_output.usb-R__DE_RODECaster_Duo_IR0037235-00.pro-output-0"},
		},
	}

	return pwpin.Config{
		Routes: []pwpin.Route{ // the higher in the list, the higher priority
			{ // link app to specific output (left channel)
				ShouldBeLinked: ptr(true),
				From: pwpin.FullyQualifiedPortSelector{
					Node: appSelector,
					Port: pwpin.Constraints{{Property: "port.name", Values: []string{"output_FL"}}},
				},
				To: pwpin.FullyQualifiedPortSelector{
					Node: sinkSelector,
					Port: pwpin.Constraints{{Property: "port.name", Values: []string{"playback_AUX0"}}},
				},
			},
			{ // link app to specific output (right channel)
				ShouldBeLinked: ptr(true),
				From: pwpin.FullyQualifiedPortSelector{
					Node: appSelector,
					Port: pwpin.Constraints{{Property: "port.name", Values: []string{"output_FR"}}},
				},
				To: pwpin.FullyQualifiedPortSelector{
					Node: sinkSelector,
					Port: pwpin.Constraints{{Property: "port.name", Values: []string{"playback_AUX1"}}},
				},
			},
			{ // ignore links to volume meters
				ShouldBeLinked: nil,
				From: pwpin.FullyQualifiedPortSelector{
					Node: appSelector,
				},
				To: pwpin.FullyQualifiedPortSelector{
					Node: pwpin.Constraints{{
						Property: "media.name",
						Values:   []string{"Peak detect"},
					}},
				},
			},
			{ // unlink app from all outputs
				ShouldBeLinked: ptr(false),
				From: pwpin.FullyQualifiedPortSelector{
					Node: appSelector,
				},
			},
		},
	}
}
