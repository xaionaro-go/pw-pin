package pwpin_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xaionaro-go/pw-pin/pkg/pwpin"
	"github.com/xaionaro-go/pw-pin/pkg/sampleconfig"
)

func TestMarshalUnmarshal(t *testing.T) {
	cfg := sampleconfig.Get()
	cfg.Routes[0].From.Node[0].Op = pwpin.ConstraintOpContains
	data, err := cfg.Bytes()
	require.NoError(t, err)

	var cfg2 pwpin.Config
	err = cfg2.Parse(data)
	require.NoError(t, err)
	require.Equal(t, cfg, cfg2)
}
