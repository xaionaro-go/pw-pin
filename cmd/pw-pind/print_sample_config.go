package main

import (
	"fmt"

	"github.com/xaionaro-go/pw-pin/pkg/sampleconfig"
)

func printSampleConfig() {
	sampleConfig := sampleconfig.Get()
	fmt.Printf("%s\n", must(sampleConfig.Bytes()))
}
