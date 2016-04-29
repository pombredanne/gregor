package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/daemons"
)

type Options struct {
	RemindServer   *rpc.FMPURI
	RemindDuration time.Duration
	MysqlDSN       string
	Debug          bool
}

const usageStr = `Usage:
notifyd -notify-server=<fmpuri> [-mysql-dsn=<user:pw@host/dbname>] [-debug]

Environment Variables

  All of the above flags have environment variable equivalents:

    -notify-server or NOTIFY_SERVER
    -mysql-dsn or MYSQL_DSN
    -debug or DEBUG
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
	var s3conf daemons.S3Config
	remindServer := &daemons.FMPURIGetter{S: os.Getenv("REMIND_SERVER")}
	mysqlDSN := &daemons.DSNGetter{S: os.Getenv("MYSQL_DSN"), S3conf: &s3conf}

	fs.StringVar(&s3conf.AWSRegion, "aws-region", os.Getenv("AWS_REGION"), "AWS region if running on AWS")
	fs.StringVar(&s3conf.ConfigBucket, "s3-config-bucket", os.Getenv("S3_CONFIG_BUCKET"), "where our S3 configs are stored")
	fs.BoolVar(&options.Debug, "debug", false, "turn on debugging")
	fs.Var(remindServer, "remind-server", "host:port of the remind server")
	fs.Var(mysqlDSN, "mysql-dsn", "user:pw@host/dbname for MySQL")

	if err := fs.Parse(argv[1:]); err != nil {
		return nil, daemons.BadUsage(err.Error())
	}

	if len(fs.Args()) != 0 {
		return nil, daemons.BadUsage("no non-flag arguments expected")
	}

	if err := s3conf.Validate(); err != nil {
		return nil, err
	}

	var ok bool
	if options.MysqlDSN, ok = mysqlDSN.Get().(string); !ok || options.MysqlDSN == "" {
		return nil, daemons.BadUsage("Error parsing mysql DSN")
	}

	if options.RemindServer, ok = remindServer.Get().(*rpc.FMPURI); !ok || options.RemindServer == nil {
		return nil, daemons.BadUsage("Error parsing session server URI")
	}

	return &options, nil
}
