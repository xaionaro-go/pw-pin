package simpleplumber_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xaionaro-go/simpleplumber/pkg/sampleconfig"
	"github.com/xaionaro-go/simpleplumber/pkg/simpleplumber"
)

func TestMarshalUnmarshal(t *testing.T) {
	cfg := sampleconfig.Get()
	cfg.Routes[0].From.Node[0].Op = simpleplumber.ConstraintOpContains
	data, err := cfg.Bytes()
	require.NoError(t, err)

	var cfg2 simpleplumber.Config
	err = cfg2.Parse(data)
	require.NoError(t, err)
	require.Equal(t, cfg, cfg2)
}
