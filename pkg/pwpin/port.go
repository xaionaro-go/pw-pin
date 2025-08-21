package pwpin

import (
	"fmt"

	pwmonitor "github.com/xaionaro-go/pipewire-monitor-go"
)

type Port struct {
	ID   int
	Info *pwmonitor.EventInfo
}

func (p *Port) String() string {
	if p.Info != nil && p.Info.Props != nil && p.Info.Props.PortName != nil {
		return fmt.Sprintf("%s:%d (%s)", p.Info.Direction, p.ID, *p.Info.Props.PortName)
	}
	return fmt.Sprintf("%s:%d", p.Info.Direction, p.ID)
}

func (p *Port) IsInput() bool {
	return p.Info.Direction == "input"
}

func (p *Port) IsOutput() bool {
	return p.Info.Direction == "output"
}
