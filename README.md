# Curate

Tooling for AWS CUR analysis.

## Setup

### Lambda and SQS

Create the Lambda, Dynamo, and SQS infrastructure:

Edit any relevant values in `cfn/parameters/lambda.json`, then:

```bash
PARAMS=lambda TEMPLATE=lambda make create-stack
```


### Add a CUR Report to the system

A CUR report is added by adding an item to the prod DynamoDB table:

```
aws dynamodb put-item --table-name curate-prod-config --item file://data/example.json
```
