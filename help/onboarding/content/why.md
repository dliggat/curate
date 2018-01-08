---
title: "Why provide access?"
date: 2017-12-27T14:11:31-08:00
anchor: "why"
weight: 1
---

The [AWS Cost and Usage Reports](http://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/billing-reports-costusage.html) provide the lowest level of granularity on your AWS spend.

One of the roles of your Technical Account Managers is to proactively look for cost-optimizations in your AWS usage. To do this more effectively your TAM needs access to your CUR reports to really dive deep into your costs to ensure you're as well optimized as possible.

The AWS TAM community have setup automatic processing of CUR reports so that TAMs can have an always up-to date and queryable version of your CUR. It's a centralized tool using standard AWS services and as such relies on a AWS IAM cross-account role to be granted access to your CUR for processing.

The tool requires read only access to the S3 bucket that contains your CUR reports. Periodically these report files are downloaded, processed, and then made available to your TAMs for querying. For access, the tool needs a IAM role to assume in your AWS account; this role has extremely limited privileges and can only access the CUR reports within the specified S3 bucket - access to any other objects in the specified bucket or any other bucket is not granted.

To help set up the required role [a Cloudformation template is provided](https://s3.amazonaws.com/aws-tam-cur-setup/customer-bucket-trust.yaml). This guide provides a walk through on spinning up a Cloudformation stack based on this template.
