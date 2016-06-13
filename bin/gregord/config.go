package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"time"

	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"github.com/keybase/gregor/bin"
	"github.com/keybase/gregor/stats"
)

type Options struct {
	AuthServer                *rpc.FMPURI
	BindAddress               string
	IncomingAddress           string
	MysqlDSN                  string
	Debug                     bool
	TLSConfig                 *tls.Config
	MockAuth                  bool
	RPCDebug                  string
	BroadcastTimeout          time.Duration
	HeartbeatInterval         time.Duration
	AliveThreshold            time.Duration
	StorageHandlers           int
	StorageQueueSize          int
	PublishBufferSize         int
	NumPublishers             int
	PublishTimeout            time.Duration
	SuperTokenRefreshInterval time.Duration
	ChildMode                 bool
	StatsBackend              stats.Backend
}

const usageStr = `Usage:
gregord -auth-server=<uri> -bind-address=[<host>]:<port> -incoming-address=[<host>]:<port> [-mysql-dsn=<user:pw@host/dbname>] [-debug]
    [-tls-key=<file|bucket|key>] [-tls-cert=<file|bucket|key>] [-aws-region=<region>] [-s3-config-bucket=<bucket>]
    [-rpc-debug=<debug str>] [-child-mode] [-tls-ca=<file|bucket|ca>] [-tls-hostname=<hostname>]

Configuring TLS

  TLS can be configured in one of the following 4 ways:
    - No TLS enabled, meaning -tls-key and -tls-cert will be unspecified
    - via AWS/S3, meaning specify -aws-region, -s3-config-bucket, -tls-key and -tls-cert. In this case
      this client will interpret the TLS key and cert as filenames to look for within the specified S3 bucket
    - via local files; in this case make -tls-key and -tls-cert look like filenames
      via the file:/// prefix.
    - via raw values; in this case, specify big ugly strings replete with newlines.

  TLS Clients
    - the -tls-ca option can be used to specify an optional root CA for TLS,
      independent of cert and key above.
    - for an anonymous host, you can use the tls-hostname to force a hostname
      (that matches the cert that the host is going to provide)

Environment Variables

  Some flags have environment variable equivalents:

    -bind-address or BIND_ADDRESS
    -incoming-address or INCOMING_ADDRESS
    -auth-server or AUTH_SERVER
    -mysql-dsn or MYSQL_DSN
    -debug or DEBUG
    -tls-key or TLS_KEY
    -tls-cert or TLS_CERT
    -tls-ca or TLS_CA
    -aws-region or AWS_REGION
    -s3-config-bucket or S3_CONFIG_BUCKET
    -rpc-debug or GREGOR_RPC_DEBUG
    --child-mode or CHILD_MODE
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
	var tlsKey, tlsCert, tlsCA, tlsHostname string
	var stathatEZKey string
	var useAwsExternalAddr bool
	authServer := &bin.FMPURIGetter{S: os.Getenv("AUTH_SERVER")}
	mysqlDSN := &bin.DSNGetter{S: os.Getenv("MYSQL_DSN"), S3conf: &s3conf}

	fs.StringVar(&options.BindAddress, "bind-address", os.Getenv("BIND_ADDRESS"), "hostname:port to bind to")
	fs.StringVar(&options.IncomingAddress, "incoming-address", os.Getenv("INCOMING_ADDRESS"), "hostname:port for external connections (will use bind-address if empty)")
	fs.BoolVar(&useAwsExternalAddr, "aws-set-external-addr", os.Getenv("AWS_SET_EXTERNAL_ADDR") != "", "set external address to EC2 local IP")
	fs.StringVar(&s3conf.AWSRegion, "aws-region", os.Getenv("AWS_REGION"), "AWS region if running on AWS")
	fs.StringVar(&s3conf.ConfigBucket, "s3-config-bucket", os.Getenv("S3_CONFIG_BUCKET"), "where our S3 configs are stored")
	fs.BoolVar(&options.Debug, "debug", os.Getenv("DEBUG") != "", "turn on debugging")
	fs.BoolVar(&options.MockAuth, "mock-auth", false, "turn on mock authentication")
	fs.BoolVar(&options.ChildMode, "child-mode", false, "exit on EOF on stdin")
	fs.StringVar(&tlsKey, "tls-key", os.Getenv("TLS_KEY"), "file or S3 bucket or raw TLS key")
	fs.StringVar(&tlsCert, "tls-cert", os.Getenv("TLS_CERT"), "file or S3 bucket or raw TLS Cert")
	fs.StringVar(&tlsCA, "tls-ca", os.Getenv("TLS_CA"), "file or S3 bucket or raw TLS CA")
	fs.StringVar(&tlsHostname, "tls-hostname", os.Getenv("TLS_HOSTNAME"), "the hostname to use with TLS")
	fs.Var(authServer, "auth-server", "host:port of the auth server")
	fs.Var(mysqlDSN, "mysql-dsn", "user:pw@host/dbname for MySQL")
	fs.StringVar(&options.RPCDebug, "rpc-debug", os.Getenv("GREGOR_RPC_DEBUG"), "RPC debug options")
	fs.DurationVar(&options.BroadcastTimeout, "broadcast-timeout", 10000*time.Millisecond,
		"Timeout on client broadcasts")
	fs.IntVar(&options.StorageHandlers, "storage-handlers", 40, "Number of threads handling storage requests")
	fs.IntVar(&options.StorageQueueSize, "storage-queue-size", 10000, "Length of the queue for requests to StorageMachine")

	fs.DurationVar(&options.HeartbeatInterval, "heartbeat-interval", 1*time.Second, "How often to send alive heartbeats")
	fs.DurationVar(&options.AliveThreshold, "alive-threshold", 2*time.Second, "Server alive threshold")
	fs.IntVar(&options.PublishBufferSize, "publish-buffer-size", 10000, "Size of the publish message buffer")
	fs.IntVar(&options.NumPublishers, "num-publishers", 10, "Number of publisher goroutines")
	fs.DurationVar(&options.PublishTimeout, "publish-timeout", 200*time.Millisecond, "Timeout on publish calls")
	fs.DurationVar(&options.SuperTokenRefreshInterval, "super-token-refresh-interval", 1*time.Hour, "Time interval in between creating new authd super tokens")
	fs.StringVar(&stathatEZKey, "stathat-ezkey", os.Getenv("STATHAT_EZKEY"), "stathat ezkey")

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

	_, port, err := net.SplitHostPort(options.BindAddress)
	if err != nil {
		return nil, bin.BadUsage("bad bind-address: %s", err)
	} else if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return nil, bin.BadUsage("bad port (%q) in bind-address: %s", port, err)
	}

	if options.TLSConfig, err = bin.ParseTLSConfig(&s3conf, tlsCert, tlsKey, tlsCA, tlsHostname); err != nil {
		return nil, err
	}

	// Use AWS instance IP if instructed for our "incoming address"
	if useAwsExternalAddr {
		// Make sure the user didn't also supply their own incoming addr
		if options.IncomingAddress != "" {
			return nil, bin.BadUsage("incoming-address specified with aws-set-external-addr")
		}
		if awsip, err := bin.GetAWSLocalIP(); err != nil {
			return nil, err
		} else {
			options.IncomingAddress = fmt.Sprintf("%s:%s", awsip, port)
			fmt.Printf("incoming: %s\n", options.IncomingAddress)
		}
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

	switch v := authServer.Get().(type) {
	case error:
		return nil, v
	case *rpc.FMPURI:
		if v == nil {
			return nil, bin.BadUsage("Error parsing auth server URI")
		}
		options.AuthServer = v
	default:
		return nil, bin.BadUsage("Error parsing auth server URI")
	}

	// Check for StatHat
	if stathatEZKey != "" {
		config := stats.NewStathatConfig(stathatEZKey, 10*time.Second)
		backend, err := stats.NewBackend(stats.STATHAT, config)
		if err != nil {
			return nil, bin.BadUsage("Error processing StatHat config: %s", err)
		}
		options.StatsBackend = backend
	}

	return &options, nil
}
