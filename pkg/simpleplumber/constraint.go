package simpleplumber

import (
	"encoding/json"
	"fmt"
	"strings"

	pwmonitor "github.com/xaionaro-go/pipewire-monitor-go"
)

type Constraint struct {
	Parameter string       `yaml:"parameter"`
	Values    []string     `yaml:"values,flow"`
	Op        ConstraintOp `yaml:"op,omitempty"`
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
		case constraintOpUndefined, ConstraintOpEquals:
			if v == valueStr {
				return true
			}
		case ConstraintOpNotEquals:
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
	ConstraintOpEquals
	ConstraintOpNotEquals
	ConstraintOpContains
	ConstraintOpNotContains
	EndOfConstraintOp
)

func (op ConstraintOp) String() string {
	switch op {
	case constraintOpUndefined:
		return "<undefined>"
	case ConstraintOpEquals:
		return "EQUALS"
	case ConstraintOpNotEquals:
		return "NOT_EQUALS"
	case ConstraintOpContains:
		return "CONTAINS"
	case ConstraintOpNotContains:
		return "NOT_CONTAINS"
	default:
		return fmt.Sprintf("unknown(%d)", op)
	}
}

func (op ConstraintOp) MarshalYAML() (any, error) {
	return op.String(), nil
}

func (op *ConstraintOp) UnmarshalYAML(unmarshal func(any) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return fmt.Errorf("failed to unmarshal constraint op: %w", err)
	}

	if str == "" {
		*op = constraintOpUndefined
		return nil
	}

	str = strings.ToUpper(str)
	for candidate := range EndOfConstraintOp {
		if candidate.String() == str {
			*op = candidate
			return nil
		}
	}

	return fmt.Errorf("unknown constraint op: %s", str)
}
