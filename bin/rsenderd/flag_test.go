package main

import (
	"testing"

	"github.com/keybase/gregor/bin"
	"github.com/stretchr/testify/require"
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
	ebu := bin.ErrBadUsage("")
	testBadUsage(t, []string{"rsenderd", "-remind-server", "XXXXX://localhost:30000",
		"-mysql-dsn", "gregor:@/gregor_test"}, ebu, "invalid framed msgpack rpc scheme")
	testBadUsage(t, []string{"rsenderd", "-remind-server", "fmprpc://localhost:30000", "-aws-region", "foo",
		"-mysql-dsn", "gregor:@/gregor_test"}, ebu, "you must provide an AWS Region and a Config bucket")
	testBadUsage(t, []string{"rsenderd", "-remind-server", "fmprpc://localhost:30000",
		"-s3-config-bucket", "foo", "-mysql-dsn", "gregor:@/gregor_test"}, ebu, "you must provide an AWS Region and a Config bucket")

	testGoodUsage(t, []string{"rsenderd", "-remind-server", "fmprpc://localhost:30000", "-mysql-dsn", "gregor:@/gregor_test"})
	testGoodUsage(t, []string{"rsenderd", "-remind-server", "fmprpc://localhost:30000", "-mysql-dsn", "gregor:@/gregor_test", "-debug"})
	testGoodUsage(t, []string{"rsenderd", "-remind-server", "fmprpc://localhost:30000", "-mysql-dsn", "gregor:@/gregor_test",
		"-aws-region", "region", "-s3-config-bucket", "bucket"})
}
