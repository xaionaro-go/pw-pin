package sampleconfig

import (
	"github.com/xaionaro-go/simpleplumber/pkg/simpleplumber"
)

var Get = func() simpleplumber.Config {
	appSelector := simpleplumber.Constraints{
		{Property: "media.name", Values: []string{" - mpv"}, Op: simpleplumber.ConstraintOpContains},
	}
	sinkSelector := simpleplumber.Constraints{
		{
			Property: "node.name",
			Values:   []string{"alsa_output.usb-R__DE_RODECaster_Duo_IR0037235-00.pro-output-0"},
		},
	}

	return simpleplumber.Config{
		Routes: []simpleplumber.Route{ // the higher in the list, the higher priority
			{ // link app to specific output (left channel)
				ShouldBeLinked: true,
				From: simpleplumber.FullyQualifiedPortSelector{
					Node: appSelector,
					Port: simpleplumber.Constraints{{Property: "port.name", Values: []string{"output_FL"}}},
				},
				To: simpleplumber.FullyQualifiedPortSelector{
					Node: sinkSelector,
					Port: simpleplumber.Constraints{{Property: "port.name", Values: []string{"playback_AUX0"}}},
				},
			},
			{ // link app to specific output (right channel)
				ShouldBeLinked: true,
				From: simpleplumber.FullyQualifiedPortSelector{
					Node: appSelector,
					Port: simpleplumber.Constraints{{Property: "port.name", Values: []string{"output_FR"}}},
				},
				To: simpleplumber.FullyQualifiedPortSelector{
					Node: sinkSelector,
					Port: simpleplumber.Constraints{{Property: "port.name", Values: []string{"playback_AUX1"}}},
				},
			},
			{ // unlink app from all outputs
				ShouldBeLinked: false,
				From: simpleplumber.FullyQualifiedPortSelector{
					Node: appSelector,
				},
			},
		},
	}
}
