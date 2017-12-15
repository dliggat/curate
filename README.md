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

A CUR report is added by adding an item to the prod DynamoDB table. `DATA` refers to a filename (without extension) in the `data/` directory.

```
DATA=example make put-ddb
```
