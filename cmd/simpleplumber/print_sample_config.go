package main

import (
	"fmt"

	"github.com/xaionaro-go/simpleplumber/pkg/sampleconfig"
)

func printSampleConfig() {
	sampleConfig := sampleconfig.Get()
	fmt.Printf("%s\n", must(sampleConfig.Bytes()))
}
