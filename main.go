package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"fmt"
	"net"
	"net/http"

	"code.cloudfoundry.org/lager"
	"github.com/alphagov/paas-go/broker"
	"github.com/alphagov/paas-s3-broker/provider"
	"github.com/alphagov/paas-s3-broker/s3"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	aws_s3 "github.com/aws/aws-sdk-go/service/s3"
)

var configFilePath string

func main() {
	flag.StringVar(&configFilePath, "config", "", "Location of the config file")
	flag.Parse()

	file, err := os.Open(configFilePath)
	if err != nil {
		log.Fatalf("Error opening config file %s: %s\n", configFilePath, err)
	}
	defer file.Close()

	config, err := broker.NewConfig(file)
	if err != nil {
		log.Fatalf("Error validating config file: %v\n", err)
	}

	err = json.Unmarshal(config.Provider, &config)
	if err != nil {
		log.Fatalf("Error parsing configuration: %v\n", err)
	}

	s3ClientConfig, err := s3.NewS3ClientConfig(config.Provider)
	if err != nil {
		log.Fatalf("Error parsing configuration: %v\n", err)
	}

	logger := lager.NewLogger("s3-service-broker")
	logger.RegisterSink(lager.NewWriterSink(os.Stdout, config.API.LagerLogLevel))

	sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(s3ClientConfig.AWSRegion)}))
	s3Client := s3.NewS3Client(s3ClientConfig, aws_s3.New(sess), iam.New(sess))

	s3Provider := provider.NewS3Provider(s3Client)
	if err != nil {
		log.Fatalf("Error creating S3 Provider: %v\n", err)
	}

	serviceBroker := broker.New(config, s3Provider, logger)
	brokerAPI := broker.NewAPI(serviceBroker, logger, config)

	listener, err := net.Listen("tcp", ":"+config.API.Port)
	if err != nil {
		log.Fatalf("Error listening to port %s: %s", config.API.Port, err)
	}
	fmt.Println("S3 Service Broker started on port " + config.API.Port + "...")
	http.Serve(listener, brokerAPI)
}
