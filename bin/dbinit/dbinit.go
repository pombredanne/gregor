package main

import (
	"flag"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/keybase/gregor/bin"
	"github.com/keybase/gregor/storage"
)

var (
	s3conf   bin.S3Config
	mysqlDSN = &bin.DSNGetter{S: os.Getenv("MYSQL_DSN"), S3conf: &s3conf}
)

func init() {
	flag.StringVar(&s3conf.AWSRegion, "aws-region", os.Getenv("AWS_REGION"), "AWS region if running on AWS")
	flag.StringVar(&s3conf.ConfigBucket, "s3-config-bucket", os.Getenv("S3_CONFIG_BUCKET"), "where our S3 configs are stored")
	flag.Var(mysqlDSN, "mysql-dsn", "user:pw@host/dbname for MySQL")
}

func main() {
	flag.Parse()
	var dsn string
	switch v := mysqlDSN.Get().(type) {
	case error:
		log.Fatal(v)
	case string:
		if v == "" {
			log.Fatal("Error parsing mysql DSN")
		}
		dsn = v
	default:
		log.Fatal("Error parsing mysql DSN")
	}

	if _, err := storage.CreateDB("mysql", dsn); err != nil {
		log.Fatal(err)
	}
}
