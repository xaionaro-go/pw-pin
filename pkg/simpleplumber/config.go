package simpleplumber

type Config struct {
	Routes Routes
}

type Routes []Route

type Route struct {
	InputNodesSelector  Constraints
	OutputNodesSelector Constraints
	ShouldBeLinked      bool
}

type Constraint struct {
	Parameter string
	Values    []string
	Op        ConstraintOp
}

type Constraints []Constraint

func (c Constraints) Match(props any) bool {
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
