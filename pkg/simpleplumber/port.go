package simpleplumber

import (
	pwmonitor "github.com/xaionaro-go/pipewire-monitor-go"
)

type Port struct {
	ID   int
	Info *pwmonitor.EventInfo
}

func (p *Port) IsInput() bool {
	return p.Info.Direction == "input"
}

func (p *Port) IsOutput() bool {
	return p.Info.Direction == "output"
}
