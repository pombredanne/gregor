package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/goamz/goamz/aws"
	"github.com/goamz/goamz/s3"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type TLSOptions struct {
	Key  string
	Cert string
}

type Options struct {
	SessionServer *url.URL
	BindAddress   string
	MysqlDSN      *url.URL
	Debug         bool
	TLSOptions    *TLSOptions
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

func readEnvOrFile(name string) (string, error) {
	doFile := false
	if strings.HasPrefix(name, "file:///") {
		name = name[7:]
		doFile = true
	} else if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "/") {
		doFile = true
	}
	if !doFile {
		return name, nil
	}
	res, err := ioutil.ReadFile(name)
	if err != nil {
		return "", badConfig("in reading file %s: %s", name, err)
	}
	return string(res), nil
}

func (o *TLSOptions) Parse(raw *rawOpts) (bool, error) {
	// Don't use TLS
	if (raw.tlsCert == "") != (raw.tlsKey == "") {
		return false, badUsage("you must provide a TLS Key and a TLS cert, or neither")
	}
	if raw.tlsCert == "" || raw.tlsKey == "" {
		return false, nil
	}
	if (raw.awsRegion == "") != (raw.configBucket == "") {
		return false, badUsage("you must provide an AWS Region and a Config bucket; can't specify one or the other")
	}
	if raw.awsRegion != "" {
		stuff, err := readFromS3Config(raw.awsRegion, raw.configBucket, raw.tlsKey, raw.tlsCert)
		if err != nil {
			return false, badConfig("error fetching TLS from S3: %s", err)
		}
		o.Key = stuff[0]
		o.Cert = stuff[1]
		return true, nil
	}
	var err error
	if o.Key, err = readEnvOrFile(raw.tlsKey); err != nil {
		return false, err
	}
	if o.Cert, err = readEnvOrFile(raw.tlsCert); err != nil {
		return false, err
	}
	return true, nil
}

func (o *Options) Parse(raw *rawOpts) error {
	if raw.helpExtended {
		usage()
		return ErrExitOnHelp
	}

	if raw.bindAddress == "" {
		return badUsage("No valid bind-address specified")
	}

	if _, port, err := net.SplitHostPort(raw.bindAddress); err != nil {
		return badUsage("bad bind-address: %s", err)
	} else if _, err = strconv.Atoi(port); err != nil {
		return badUsage("bad port in bind-address: %s", err)
	}

	o.BindAddress = raw.bindAddress

	if raw.sessionServerURI == "" {
		return badUsage("No session-server URI specified")
	}

	var err error
	if o.SessionServer, err = url.Parse(raw.sessionServerURI); err != nil {
		return badUsage("Error parsing session-server: %s", err)
	}

	o.Debug = raw.debug

	if raw.mysqlDSN != "" {
		if o.MysqlDSN, err = url.Parse(raw.mysqlDSN); err != nil {
			return badUsage("Error parsing mysql DSN: %s", err)
		}
	}

	var tlsOpts TLSOptions
	if used, err := tlsOpts.Parse(raw); err != nil {
		return err
	} else if used {
		o.TLSOptions = &tlsOpts
	}
	return nil
}

// ReadFromS3Config reads the content of the files denoted by fileNames
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
	sessionServerURI string
	bindAddress      string
	mysqlDSN         string
	debug            bool
	tlsKey           string
	tlsCert          string
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
	fs.StringVar(&raw.sessionServerURI, "session-server", os.Getenv("SESSION_SERVER"), "host:port of the session server")
	fs.StringVar(&raw.bindAddress, "bind-address", os.Getenv("BIND_ADDRESS"), "hostname:port to bind to")
	fs.StringVar(&raw.mysqlDSN, "mysql-dsn", os.Getenv("MYSQL_DSN"), "user:pw@host/dbname for MySQL")
	fs.BoolVar(&raw.debug, "debug", false, "turn on debugging")
	fs.StringVar(&raw.tlsKey, "tls-key", os.Getenv("TLS_KEY"), "file or S3 bucket or raw TLS key")
	fs.StringVar(&raw.tlsCert, "tls-cert", os.Getenv("TLS_CERT"), "file or S3 bucket or raw TLS Cert")
	fs.StringVar(&raw.awsRegion, "aws-region", os.Getenv("AWS_REGION"), "AWS region if running on AWS")
	fs.StringVar(&raw.configBucket, "s3-config-bucket", os.Getenv("S3_CONFIG_BUCKET"), "where our S3 configs are stored")
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
