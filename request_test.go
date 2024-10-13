package gemini_test

import (
	"testing"

	"github.com/knowfox/gemini"
	"github.com/stretchr/testify/require"
)

func TestReset(t *testing.T) {
	r := &gemini.Request{}
	err := r.Reset(nil, "titan://some-hostname.com:1965/da;mime=text/plain;size=23")
	require.NoError(t, err)
	require.Equal(t, "/da", r.URL.Path)
	require.Equal(t, "text/plain", r.Titan.Mime)
	require.Equal(t, int64(23), r.Titan.Size)
	require.Equal(t, "", r.Titan.Token)
}
