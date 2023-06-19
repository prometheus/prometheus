package server_test

// import (
// 	"context"
// 	"testing"

// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"

// 	"github.com/prometheus/prometheus/pp/go/server"
// 	"github.com/prometheus/prometheus/pp/go/transport"
// )

// func TestFileHandler(t *testing.T) {
// 	prefixDir := "testDir-"
// 	prefixFile := "testFile-"
// 	payload := []byte{1, 2, 3}

// 	fh, err := server.NewFileHandler(prefixDir, prefixFile)
// 	require.NoError(t, err)
// 	defer fh.Remove()
// 	defer fh.Close()

// 	wrraw := transport.NewRawMessage(
// 		3,
// 		transport.MsgPut,
// 		payload,
// 	)
// 	err = fh.Write(wrraw)
// 	require.NoError(t, err)

// 	err = fh.Sync()
// 	require.NoError(t, err)

// 	err = fh.SeekStart()
// 	require.NoError(t, err)

// 	rraw, err := fh.Next(context.TODO())
// 	require.NoError(t, err)

// 	assert.Equal(t, wrraw.Header, rraw.Header)
// 	assert.Equal(t, wrraw.Payload, rraw.Payload)
// }
