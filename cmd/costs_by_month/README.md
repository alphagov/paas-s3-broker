# costs_by_month

## Overview

This is a tool for outputting the cost of each tenant S3 bucket. It
breaks costs down by organisation and space as well as service
instance, and provides costs for each of the last 3 months.

## Build

```
go build -o costs_by_month
```

## Run

1. Set AWS access credentials in your shell environment for the AWS
   Account hosting all the tenant S3 buckets;
2. `cf login` to the Cloud Foundry in which the tenant S3 bucket
   service instances exist;
3. ```
   ./costs_by_month \
     --cf-api-url https://api.cloud.service.gov.uk \
     --cf-api-token "$(cf oauth-token)"
   ```
