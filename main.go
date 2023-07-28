package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"log"
	"os"

	"fmt"
	"net"
	"net/http"

	"code.cloudfoundry.org/lager"
	"github.com/alphagov/paas-s3-broker/provider"
	"github.com/alphagov/paas-s3-broker/s3"
	"github.com/alphagov/paas-service-broker-base/broker"
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
	s3Client := s3.NewS3Client(s3ClientConfig, aws_s3.New(sess), iam.New(sess), logger, context.Background())

	s3Provider := provider.NewS3Provider(s3Client)
	if err != nil {
		log.Fatalf("Error creating S3 Provider: %v\n", err)
	}

	serviceBroker, err := broker.New(config, s3Provider, logger)
	if err != nil {
		log.Fatalf("Error creating service broker: %s", err)
	}

	brokerAPI := broker.NewAPI(serviceBroker, logger, config)

	listenAddress := fmt.Sprintf("%s:%s", config.API.Host, config.API.Port)
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Fatalf("Error listening to port %s: %s", config.API.Port, err)
	}
	if config.API.TLSEnabled() {
		tlsConfig, err := config.API.TLS.GenerateTLSConfig()
		if err != nil {
			log.Fatalf("Error configuring TLS: %s", err)
		}
		listener = tls.NewListener(listener, tlsConfig)
		fmt.Printf("S3 Service Broker started https://%s...\n", listenAddress)
	} else {
		fmt.Printf("S3 Service Broker started http://%s...\n", listenAddress)
	}
	http.Serve(listener, brokerAPI)
}
