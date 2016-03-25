package main

import (
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

func testBadUsage(t *testing.T, args []string, wantedErr error, msg string) {
	opts, err := ParseOptionsQuiet(args)
	require.Nil(t, opts, "no options returned")
	require.NotNil(t, err, "Error object returned")
	require.IsType(t, wantedErr, err, "right type")
	require.Contains(t, err.Error(), msg, "bad msg")
}

func TestUsage(t *testing.T) {
	testBadUsage(t, []string{"gregor"}, ErrBadUsage(""), "No valid listen port")
	testBadUsage(t, []string{"gregor", "--port", "aabb"}, errors.New(""), "invalid syntax")
	testBadUsage(t, []string{"gregor", "--port", "4000"}, ErrBadUsage(""), "No session-server URI specified")
	testBadUsage(t, []string{"gregor", "--port", "4000", "--session-server", "localhost", "--tls-key", "hi"},
		ErrBadUsage(""), "you must provide a TLS Key and a TLS cert, or neither")
	testBadUsage(t, []string{"gregor", "--port", "4000", "--session-server", "localhost", "--tls-cert", "hi"},
		ErrBadUsage(""), "you must provide a TLS Key and a TLS cert, or neither")
	testBadUsage(t, []string{"gregor", "--port", "4000", "--session-server", "localhost", "--tls-key", "hi",
		"--tls-cert", "bye", "--aws-region", "foo"}, ErrBadUsage(""), "you must provide an AWS Region and a Config bucket")
	testBadUsage(t, []string{"gregor", "--port", "4000", "--session-server", "localhost", "--tls-key", "hi",
		"--tls-cert", "bye", "--s3-config-bucket", "foo"}, ErrBadUsage(""), "you must provide an AWS Region and a Config bucket")
	testBadUsage(t, []string{"gregor", "--port", "4000", "--session-server", "localhost", "--tls-key", "hi",
		"--tls-cert", "file:///does/not/exist"}, ErrBadConfig(""), "no such file or directory")
}
