.PHONY: _check-params _check-template create-stack update-stack delete-stack describe-stack build
BINARY = processor

# Read the cloudformation/parameters.json file for the ProjectName and EnvionmentName.
# Use these to name the CloudFormation stack.
PROJECT_NAME = $(shell cat cfn/parameters/$(PARAMS).json | python -c 'import sys, json; j = [i for i in json.load(sys.stdin) if i["ParameterKey"]=="ProjectName"][0]["ParameterValue"]; print j')
ENVIRONMENT_NAME = $(shell cat cfn/parameters/$(PARAMS).json | python -c 'import sys, json; j = [i for i in json.load(sys.stdin) if i["ParameterKey"]=="EnvironmentName"][0]["ParameterValue"]; print j')
STACK_NAME = $(PROJECT_NAME)-$(ENVIRONMENT_NAME)-$(TEMPLATE)-stack

_check-params:
ifndef PARAMS
	$(error PARAMS is undefined; set to the appropriate filename from cfn/parameters/ (without extension) )
endif

_check-template:
ifndef TEMPLATE
	$(error TEMPLATE is undefined; set to the appropriate file from cfn/templates/ (without extension) )
endif

create-stack: _check-params _check-template
	aws cloudformation create-stack \
	  --stack-name $(STACK_NAME) \
	  --template-body file://cfn/templates/$(TEMPLATE).yaml \
	  --parameters file://cfn/parameters/$(PARAMS).json \
	  --capabilities CAPABILITY_IAM

update-stack: _check-params _check-template
	aws cloudformation update-stack \
	  --stack-name $(STACK_NAME) \
	  --template-body file://cfn/templates/$(TEMPLATE).yaml \
	  --parameters file://cfn/parameters/$(PARAMS).json \
	  --capabilities CAPABILITY_IAM

delete-stack: _check-params _check-template
	aws cloudformation delete-stack \
	  --stack-name $(STACK_NAME)

describe-stack: _check-params _check-template
	aws cloudformation describe-stacks \
	  --stack-name $(STACK_NAME)

build:
	GOOS=linux GOARCH=amd64 go build $(BINARY).go
	mv $(BINARY) bin/$(BINARY)
