package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/bin"
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
	var s3conf bin.S3Config
	remindServer := &bin.FMPURIGetter{S: os.Getenv("REMIND_SERVER")}
	mysqlDSN := &bin.DSNGetter{S: os.Getenv("MYSQL_DSN"), S3conf: &s3conf}

	fs.StringVar(&s3conf.AWSRegion, "aws-region", os.Getenv("AWS_REGION"), "AWS region if running on AWS")
	fs.StringVar(&s3conf.ConfigBucket, "s3-config-bucket", os.Getenv("S3_CONFIG_BUCKET"), "where our S3 configs are stored")
	fs.BoolVar(&options.Debug, "debug", os.Getenv("DEBUG") != "", "turn on debugging")
	fs.DurationVar(&options.RemindDuration, "remind-duration", 5*time.Minute, "interval between sending reminders")
	fs.Var(remindServer, "remind-server", "host:port of the remind server")
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

	switch v := mysqlDSN.Get().(type) {
	case error:
		return nil, v
	case string:
		if v == "" {
			return nil, bin.BadUsage("Error parsing mysql DSN")
		}
		options.MysqlDSN = v
	default:
		return nil, bin.BadUsage("Error parsing mysql DSN")
	}

	switch v := remindServer.Get().(type) {
	case error:
		return nil, v
	case *rpc.FMPURI:
		if v == nil {
			return nil, bin.BadUsage("Error parsing mysql DSN")
		}
		options.RemindServer = v
	default:
		return nil, bin.BadUsage("Error parsing mysql DSN")
	}

	return &options, nil
}
