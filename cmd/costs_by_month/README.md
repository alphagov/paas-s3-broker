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
     --cf-api-token "$(cf oauth-token)" \
     --cf-api-url https://api.london.cloud.service.gov.uk
   ```

That should output something like:

```
Service instances with an empty NAME and ORG were found in AWS Cost Explorer but not in Cloud Foundry. Commonly this means they have been deleted.

+--------------------------------------+-----------------------+----------------------------+----------------+------------------+------------+------------+
|        SERVICE INSTANCE GUID         |         NAME          |            ORG             |   2019-05-01   |    2019-04-01    | 2019-03-01 | 2019-02-01 |
+--------------------------------------+-----------------------+----------------------------+----------------+------------------+------------+------------+
| 05778377-355c-4089-886b-faae00d8744c | fake-name-of-instance | fake-name-of-organisation  |                | 0.0000462 USD    |            |            |
| 89a4fb55-e1b9-4b8a-81ff-e84ed4505789 |                       |                            |                | 0.00005396 USD   |            |            |
| 91006dd2-61b3-41a5-ae66-a67888be02d5 |                       |                            |                | 0.0000512 USD    |            |            |
| 94e1c4d7-6970-4edb-9d04-c878c13a6c36 | another-fake-name     | another-fake-org           |                | 0.00005396 USD   |            |            |
| a06203a5-8c6c-4353-a5bc-0a264aee1513 |                       |                            |                | 0.00004866 USD   |            |            |
| b2264d99-52af-43fa-be2b-47bcb0494404 |                       |                            |                | 0.00004294 USD   |            |            |
+--------------------------------------+-----------------------+----------------------------+----------------+------------------+------------+------------+
```

⭐ You can also output CSV by providing the `--use-csv` command line flag.
