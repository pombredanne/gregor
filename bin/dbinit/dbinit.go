package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/keybase/gregor/bin"
	"github.com/keybase/gregor/schema"
)

var (
	skipSanityCheck bool
	skipExistsCheck bool
	dsnVar          = "MYSQL_DSN"
	s3conf          bin.S3Config
	mysqlDSN        = &bin.DSNGetter{S: os.Getenv(dsnVar), S3conf: &s3conf}
)

func init() {
	flag.BoolVar(&skipSanityCheck, "y", false, "skip confirmation and init db")
	flag.BoolVar(&skipExistsCheck, "force", false, "create the database even if tables exist")
	flag.StringVar(&s3conf.AWSRegion, "aws-region", os.Getenv("AWS_REGION"), "AWS region if running on AWS")
	flag.StringVar(&s3conf.ConfigBucket, "s3-config-bucket", os.Getenv("S3_CONFIG_BUCKET"), "where our S3 configs are stored")
	flag.Var(mysqlDSN, "mysql-dsn", "user:pw@host/dbname for MySQL")
}

func main() {
	if len(os.Getenv(dsnVar)) == 0 {
		log.Fatalf("Error, environment variable %s is not defined.", dsnVar)
	}
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

	if !skipSanityCheck {
		fmt.Println("Creating/Reseting DB at", dsn)
		fmt.Println("This will lead to irreversible loss of data currently in the DB.")
		fmt.Print("Are you sure? (Y/N): ")
		reader := bufio.NewReader(os.Stdin)
		resp, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}

		resp = strings.TrimSuffix(strings.ToUpper(resp), "\n")
		switch resp {
		case "Y":
			fmt.Println("Creating DB...")
		case "N":
			log.Fatal("Chose not to create DB.")
		default:
			log.Fatal("Invalid response")
		}
	}

	// Check if the gregor tables already exist, unless we are just charging forward
	if !skipExistsCheck {
		exists, err := schema.DBExists("mysql", dsn)
		if err != nil {
			log.Fatal(err)
		}
		if exists {
			log.Fatal("database already exists, exiting...")
		}
	}

	if _, err := schema.CreateDB("mysql", dsn); err != nil {
		log.Fatal(err)
	}
}
