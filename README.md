# Curate

Tooling for AWS CUR analysis.

## Setup

### Lambda and SQS

Create the Lambda, Dynamo, and SQS infrastructure:

Edit any relevant values in `cfn/parameters/lambda.json`, then:

```bash
PARAMS=lambda TEMPLATE=lambda make create-stack
```
