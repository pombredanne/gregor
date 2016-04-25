package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/goamz/goamz/aws"
	"github.com/goamz/goamz/s3"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
)

type Options struct {
	RemindServer   *rpc.FMPURI
	RemindDuration time.Duration
	MysqlDSN       *url.URL
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

type ErrBadUsage string
type ErrBadConfig string

var ErrExitOnHelp = errors.New("exit on help request")

func badUsage(f string, args ...interface{}) error {
	return ErrBadUsage(fmt.Sprintf(f, args...))
}

func badConfig(f string, args ...interface{}) error {
	return ErrBadConfig(fmt.Sprintf(f, args...))
}

func (e ErrBadUsage) Error() string  { return "bad usage: " + string(e) }
func (e ErrBadConfig) Error() string { return "bad config: " + string(e) }

func usage() {
	warnf("%s", usageStr)
}

func warnf(f string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, f, args...)
}

func errorf(f string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "error: "+f, args...)
}

func parseMysqlDSN(raw *rawOpts) (*url.URL, error) {
	var dsn string
	if len(raw.awsRegion) == 0 {
		dsn = raw.mysqlDSN
	} else {
		results, err := readFromS3Config(raw.awsRegion, raw.configBucket, raw.mysqlDSN)
		if err != nil {
			return nil, err
		}
		dsn = strings.TrimSpace(results[0])
		if len(dsn) == 0 {
			return nil, fmt.Errorf("empty dsn")
		}
	}
	return url.Parse(dsn)
}

func (o *Options) Parse(raw *rawOpts) error {
	if raw.helpExtended {
		usage()
		return ErrExitOnHelp
	}

	if raw.remindServerAddr == "" {
		return badUsage("No session-server URI specified")
	}

	var err error
	if o.RemindServer, err = rpc.ParseFMPURI(raw.remindServerAddr); err != nil {
		return badUsage("Error parsing session-server: %s", err)
	}
	o.RemindDuration = raw.remindDuration

	o.Debug = raw.debug

	if raw.mysqlDSN != "" {
		if o.MysqlDSN, err = parseMysqlDSN(raw); err != nil {
			return badUsage("Error parsing mysql DSN: %s", err)
		}
	} else {
		return badUsage("No mysql-dsn specified")
	}

	return nil
}

// readFromS3Config reads the content of the files denoted by fileNames
// from S3 configuration bucket
func readFromS3Config(region string, bucket string, fileNames ...string) ([]string, error) {
	if _, ok := aws.Regions[region]; !ok {
		return nil, badConfig("unknown region: %s", region)
	}
	// this will attempt to populate an Auth object by getting
	// credentials from (in order):
	//
	//   (1) credentials file
	//   (2) environment variables
	//   (3) instance role (this will be the case for production)
	//
	auth, err := aws.GetAuth("", "", "", time.Time{})
	if err != nil {
		return nil, err
	}
	client := s3.New(auth, aws.Regions[region]).Bucket(bucket)
	results := make([]string, len(fileNames))
	for i, name := range fileNames {
		buf, err := client.Get(name)
		if err != nil {
			return nil, err
		}
		results[i] = string(buf)
	}
	return results, nil
}

type rawOpts struct {
	remindServerAddr string
	remindDuration   time.Duration
	mysqlDSN         string
	debug            bool
	awsRegion        string
	configBucket     string
	helpExtended     bool
}

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
	}
	var raw rawOpts
	fs.StringVar(&raw.remindServerAddr, "remind-server", os.Getenv("REMIND_SERVER"), "FMPURI of the remind server")
	fs.DurationVar(&raw.remindDuration, "remind-duration", time.Minute, "Duration between remind calls")
	fs.StringVar(&raw.mysqlDSN, "mysql-dsn", os.Getenv("MYSQL_DSN"), "user:pw@host/dbname for MySQL")
	fs.BoolVar(&raw.debug, "debug", false, "turn on debugging")
	fs.BoolVar(&raw.helpExtended, "help-extended", false, "get more help")

	if err := fs.Parse(argv[1:]); err != nil {
		return nil, err
	}

	if len(fs.Args()) != 0 {
		return nil, badUsage("no non-flag arguments expected")
	}

	var options Options
	if err := options.Parse(&raw); err != nil {
		return nil, err
	}
	return &options, nil
}
