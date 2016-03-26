package main

import (
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

func testGoodUsage(t *testing.T, args []string) {
	opts, err := ParseOptionsQuiet(args)
	require.NotNil(t, opts, "opts came back")
	require.Nil(t, err, "no error")
}

func TestUsage(t *testing.T) {
	ebu := ErrBadUsage("")
	testBadUsage(t, []string{"gregor"}, ebu, "No valid bind-address specified")
	testBadUsage(t, []string{"gregor", "--bind-address", "aabb"}, ebu, "bad bind-address")
	testBadUsage(t, []string{"gregor", "--bind-address", "localhost:aabb", "--session-server", "localhost"}, ebu, "bad port in bind-address")
	testBadUsage(t, []string{"gregor", "--bind-address", ":4000"}, ebu, "No session-server URI specified")
	testBadUsage(t, []string{"gregor", "--bind-address", ":4000", "--session-server", "localhost", "--tls-key", "hi"},
		ebu, "you must provide a TLS Key and a TLS cert, or neither")
	testBadUsage(t, []string{"gregor", "--bind-address", ":4000", "--session-server", "localhost", "--tls-cert", "hi"},
		ebu, "you must provide a TLS Key and a TLS cert, or neither")
	testBadUsage(t, []string{"gregor", "--bind-address", ":4000", "--session-server", "localhost", "--tls-key", "hi",
		"--tls-cert", "bye", "--aws-region", "foo"}, ebu, "you must provide an AWS Region and a Config bucket")
	testBadUsage(t, []string{"gregor", "--bind-address", ":4000", "--session-server", "localhost", "--tls-key", "hi",
		"--tls-cert", "bye", "--s3-config-bucket", "foo"}, ebu, "you must provide an AWS Region and a Config bucket")
	testBadUsage(t, []string{"gregor", "--bind-address", ":4000", "--session-server", "localhost", "--tls-key", "hi",
		"--tls-cert", "file:///does/not/exist"}, ErrBadConfig(""), "no such file or directory")

	testGoodUsage(t, []string{"gregor", "--session-server", "localhost", "--bind-address", ":4000"})
	testGoodUsage(t, []string{"gregor", "--session-server", "localhost", "--bind-address", "127.0.0.1:4000"})
	testGoodUsage(t, []string{"gregor", "--session-server", "localhost", "--bind-address", "0.0.0.0:4000"})
	testGoodUsage(t, []string{"gregor", "--session-server", "localhost", "--bind-address", "localhost:4000"})
}
