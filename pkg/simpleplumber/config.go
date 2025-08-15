package simpleplumber

import (
	"encoding/json"
	"fmt"
	"strings"

	pwmonitor "github.com/ConnorsApps/pipewire-monitor-go"
)

type Config struct {
	Routes Routes
}

type Routes []Route

type Route struct {
	InputNodesSelector  Constraints
	InputPortsSelector  Constraints
	OutputNodesSelector Constraints
	OutputPortsSelector Constraints
	ShouldBeLinked      bool
}

type Constraint struct {
	Parameter string
	Values    []string
	Op        ConstraintOp
}

func (c Constraint) Match(props pwmonitor.EventInfoProps) bool {
	if c.Parameter == "" {
		return false
	}

	b, err := json.Marshal(props)
	if err != nil {
		panic(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		panic(err)
	}

	value, ok := m[c.Parameter]
	if !ok {
		return false
	}
	valueStr := fmt.Sprintf("%v", value)

	for _, v := range c.Values {
		switch c.Op {
		case ConstraintOpEqual:
			if v == valueStr {
				return true
			}
		case ConstraintOpNotEqual:
			if v != valueStr {
				return true
			}
		case ConstraintOpContains:
			if strings.Contains(valueStr, v) {
				return true
			}
		case ConstraintOpNotContains:
			if !strings.Contains(valueStr, v) {
				return true
			}
		default:
			return false
		}
	}
	return false
}

type Constraints []Constraint

func (c Constraints) Match(props pwmonitor.EventInfoProps) bool {
	for _, constraint := range c {
		if !constraint.Match(props) {
			return false
		}
	}
	return true
}

type ConstraintOp int

const (
	constraintOpUndefined ConstraintOp = iota
	ConstraintOpEqual
	ConstraintOpNotEqual
	ConstraintOpContains
	ConstraintOpNotContains
)
