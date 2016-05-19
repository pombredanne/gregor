package main

import (
	"os"
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
	testBadUsage(t, []string{"gregor"}, ebu, "No valid bind-address specified")
	testBadUsage(t, []string{"gregor", "-bind-address", "aabb"}, ebu, "bad bind-address: missing port in address aabb")
	testBadUsage(t, []string{"gregor", "-bind-address", "localhost:aabb"}, ebu, "bad port (\"aabb\") in bind-address")
	testBadUsage(t, []string{"gregor", "-bind-address", "localhost:65537"}, ebu, "bad port (\"65537\") in bind-address")
	testBadUsage(t, []string{"gregor", "-bind-address", "localhost:-20"}, ebu, "bad port (\"-20\") in bind-address")
	testBadUsage(t, []string{"gregor", "-bind-address", ":4000", "-mysql-dsn", "gregor:@/gregor_test"}, ebu, "Error parsing auth server URI")
	testBadUsage(t, []string{"gregor", "-bind-address", ":4000", "-mysql-dsn", "gregor:@/gregor_test", "-auth-server", "XXXXX://localhost:30000"}, ebu, "invalid framed msgpack rpc scheme")
	testBadUsage(t, []string{"gregor", "-bind-address", ":4000", "-mysql-dsn", "gregor:@/gregor_test", "-auth-server", "fmprpc://localhost:30000", "-tls-key", "hi"},
		ebu, "you must provide a TLS Key and a TLS cert, or neither")
	testBadUsage(t, []string{"gregor", "-bind-address", ":4000", "-mysql-dsn", "gregor:@/gregor_test", "-auth-server", "fmprpc://localhost:30000", "-tls-cert", "hi"},
		ebu, "you must provide a TLS Key and a TLS cert, or neither")
	testBadUsage(t, []string{"gregor", "-bind-address", ":4000", "-auth-server", "fmprpc://localhost:30000", "-tls-key", "hi",
		"-tls-cert", "bye", "-aws-region", "foo", "-mysql-dsn", "gregor:@/gregor_test"}, ebu, "you must provide an AWS Region and a Config bucket")
	testBadUsage(t, []string{"gregor", "-bind-address", ":4000", "-auth-server", "fmprpc://localhost:30000", "-tls-key", "hi",
		"-tls-cert", "bye", "-s3-config-bucket", "foo", "-mysql-dsn", "gregor:@/gregor_test"}, ebu, "you must provide an AWS Region and a Config bucket")
	testBadUsage(t, []string{"gregor", "-bind-address", ":4000", "-auth-server", "fmprpc://localhost:30000", "-tls-key", "hi",
		"-tls-cert", "file:///does/not/exist", "-mysql-dsn", "gregor:@/gregor_test"}, bin.ErrBadConfig(""), "no such file or directory")

	testGoodUsage(t, []string{"gregor", "-auth-server", "fmprpc://localhost:30000", "-bind-address", ":4000", "-mysql-dsn", "gregor:@/gregor_test"})
	testGoodUsage(t, []string{"gregor", "-auth-server", "fmprpc://localhost:30000", "-bind-address", "127.0.0.1:4000", "-mysql-dsn", "gregor:@/gregor_test"})
	testGoodUsage(t, []string{"gregor", "-auth-server", "fmprpc://localhost:30000", "-bind-address", "0.0.0.0:4000", "-mysql-dsn", "gregor:@/gregor_test"})
	testGoodUsage(t, []string{"gregor", "-auth-server", "fmprpc://localhost:30000", "-bind-address", "localhost:4000", "-mysql-dsn", os.Getenv("MYSQL_DSN")})
}
