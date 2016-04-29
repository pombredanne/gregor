package daemons

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"strings"
	"time"

	"github.com/goamz/goamz/aws"
	"github.com/goamz/goamz/s3"
	rpc "github.com/keybase/go-framed-msgpack-rpc"
)

type ErrBadUsage string
type ErrBadConfig string

var ErrExitOnHelp = errors.New("exit on help request")

func BadUsage(f string, args ...interface{}) error {
	return ErrBadUsage(fmt.Sprintf(f, args...))
}

func BadConfig(f string, args ...interface{}) error {
	return ErrBadConfig(fmt.Sprintf(f, args...))
}

func (e ErrBadUsage) Error() string  { return "bad usage: " + string(e) }
func (e ErrBadConfig) Error() string { return "bad config: " + string(e) }

// ReadFromS3Config reads the content of the files denoted by fileNames
// from S3 configuration bucket
func readFromS3Config(s3conf *S3Config, name string) ([]byte, error) {
	if err := s3conf.Validate(); err != nil {
		return nil, err
	}

	if _, ok := aws.Regions[s3conf.AWSRegion]; !ok {
		return nil, BadConfig("unknown region: %s", s3conf.AWSRegion)
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
	client := s3.New(auth, aws.Regions[s3conf.AWSRegion]).Bucket(s3conf.ConfigBucket)
	buf, err := client.Get(name)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func ReadAWSEnvOrFile(s3conf *S3Config, name string) ([]byte, error) {
	if s3conf.AWSRegion != "" {
		return readFromS3Config(s3conf, name)
	}
	if strings.HasPrefix(name, "file:///") {
		return ioutil.ReadFile(name[7:])
	} else if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "/") {
		return ioutil.ReadFile(name)
	}
	return []byte(name), nil
}

func ParseTLSConfig(s3conf *S3Config, tlsCert, tlsKey string) (*tls.Config, error) {
	if (tlsCert == "") != (tlsKey == "") {
		return nil, BadUsage("you must provide a TLS Key and a TLS cert, or neither")
	}
	if tlsCert == "" || tlsKey == "" {
		return nil, nil
	}

	cert, err := ReadAWSEnvOrFile(s3conf, tlsCert)
	if err != nil {
		return nil, BadConfig("error fetching TLS cert from S3: %s", err)
	}

	key, err := ReadAWSEnvOrFile(s3conf, tlsKey)
	if err != nil {
		return nil, BadConfig("error fetching TLS key from S3: %s", err)
	}

	x509kp, err := tls.X509KeyPair([]byte(cert), []byte(key))
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{x509kp}}, nil

}

type S3Config struct {
	AWSRegion, ConfigBucket string
}

func (s3conf *S3Config) Validate() (err error) {
	if (s3conf.AWSRegion == "") != (s3conf.ConfigBucket == "") {
		err = BadUsage("you must provide an AWS Region and a Config bucket; can't specify one or the other")
	}
	return
}

type FMPURIGetter struct {
	S   string
	val *rpc.FMPURI
}

func (v *FMPURIGetter) Get() interface{} {
	if v.val == nil && v.S != "" {
		if err := v.Set(v.S); err != nil {
			return err
		}
	}
	return v.val
}

func (v *FMPURIGetter) Set(s string) error {
	v.S = s
	val, err := rpc.ParseFMPURI(s)
	if err != nil {
		return err
	}
	v.val = val
	return nil
}

func (v *FMPURIGetter) String() string {
	return v.S
}

type DSNGetter struct {
	S      string
	S3conf *S3Config
	val    *url.URL
}

func (v *DSNGetter) Get() interface{} {
	if v.val == nil && v.S != "" {
		if err := v.Set(v.S); err != nil {
			return err
		}
	}
	return v.val
}

func (v *DSNGetter) Set(s string) error {
	v.S = s
	if v.S3conf.AWSRegion != "" {
		b, err := readFromS3Config(v.S3conf, s)
		if err != nil {
			return err
		}
		s = strings.TrimSpace(string(b))
	}
	val, err := url.Parse(s)
	if err != nil {
		return err
	}
	v.val = val
	return nil
}

func (v *DSNGetter) String() string {
	return v.S
}
