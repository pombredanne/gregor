package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/bin"
)

type Options struct {
	SessionServer *rpc.FMPURI
	BindAddress   string
	MysqlDSN      string
	Debug         bool
	TLSConfig     *tls.Config
	MockAuth      bool
}

const usageStr = `Usage:
gregord -session-server=<uri> -bind-address=[<host>]:<port> [-mysql-dsn=<user:pw@host/dbname>] [-debug]
    [-tls-key=<file|bucket|key>] [-tls-cert=<file|bucket|key>] [-aws-region=<region>] [-s3-config-bucket=<bucket>]

Configuring TLS

  TLS can be configured in one of the following 4 ways:
    - No TLS enabled, meaning -tls-key and -tls-cert will be unspecified
    - via AWS/S3, meaning specify -aws-region, -s3-config-bucket, -tls-key and -tls-cert. In this case
      this client will interpret the TLS key and cert as filenames to look for within the specified S3 bucket
    - via local files; in this case make -tls-key and -tls-cert look like filenames
      via the file:/// prefix.
    - via raw values; in this case, specify big ugly strings replete with newlines.

Environment Variables

  All of the above flags have environment variable equivalents:

    -bind-address or BIND_ADDRESS
    -session-server or SESSION_SERVER
    -mysql-dsn or MYSQL_DSN
    -debug or DEBUG
    -tls-key or TLS_KEY
    -tls-cert or TLS_CERT
    -aws-region or AWS_REGION
    -s3-config-bucket or S3_CONFIG_BUCKET
`

func ParseOptions(argv []string) (*Options, error) {
	return parseOptions(argv, false)
}

func ParseOptionsQuiet(argv []string) (*Options, error) {
	return parseOptions(argv, true)
}

func parseOptions(argv []string, quiet bool) (*Options, error) {
	fs := flag.NewFlagSet(argv[0], flag.ContinueOnError)
	if quiet {
		fs.Usage = func() {}
		fs.SetOutput(ioutil.Discard)
	} else {
		fs.Usage = func() { fmt.Fprint(os.Stderr, usageStr) }
	}

	var options Options
	var s3conf bin.S3Config
	var tlsKey, tlsCert string
	sessionServer := &bin.FMPURIGetter{S: os.Getenv("SESSION_SERVER")}
	mysqlDSN := &bin.DSNGetter{S: os.Getenv("MYSQL_DSN"), S3conf: &s3conf}

	fs.StringVar(&options.BindAddress, "bind-address", os.Getenv("BIND_ADDRESS"), "hostname:port to bind to")
	fs.StringVar(&s3conf.AWSRegion, "aws-region", os.Getenv("AWS_REGION"), "AWS region if running on AWS")
	fs.StringVar(&s3conf.ConfigBucket, "s3-config-bucket", os.Getenv("S3_CONFIG_BUCKET"), "where our S3 configs are stored")
	fs.BoolVar(&options.Debug, "debug", false, "turn on debugging")
	fs.BoolVar(&options.MockAuth, "mock-auth", false, "turn on mock authentication")
	fs.StringVar(&tlsKey, "tls-key", os.Getenv("TLS_KEY"), "file or S3 bucket or raw TLS key")
	fs.StringVar(&tlsCert, "tls-cert", os.Getenv("TLS_CERT"), "file or S3 bucket or raw TLS Cert")
	fs.Var(sessionServer, "session-server", "host:port of the session server")
	fs.Var(mysqlDSN, "mysql-dsn", "user:pw@host/dbname for MySQL")

	if err := fs.Parse(argv[1:]); err != nil {
		return nil, bin.BadUsage(err.Error())
	}

	if len(fs.Args()) != 0 {
		return nil, bin.BadUsage("no non-flag arguments expected")
	}

	if err := s3conf.Validate(); err != nil {
		return nil, err
	}

	if options.BindAddress == "" {
		return nil, bin.BadUsage("No valid bind-address specified")
	}

	if _, port, err := net.SplitHostPort(options.BindAddress); err != nil {
		return nil, bin.BadUsage("bad bind-address: %s", err)
	} else if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return nil, bin.BadUsage("bad port (%q) in bind-address: %s", port, err)
	}

	var err error
	if options.TLSConfig, err = bin.ParseTLSConfig(&s3conf, tlsCert, tlsKey); err != nil {
		return nil, err
	}

	var ok bool
	if options.MysqlDSN, ok = mysqlDSN.Get().(string); !ok || options.MysqlDSN == "" {
		return nil, bin.BadUsage("Error parsing mysql DSN")
	}

	if options.SessionServer, ok = sessionServer.Get().(*rpc.FMPURI); !ok || options.SessionServer == nil {
		return nil, bin.BadUsage("Error parsing session server URI")
	}

	return &options, nil
}
