---
title: "What about security?"
date: 2017-12-27T14:11:41-08:00
anchor: "security"
weight: 3
---

Data security at AWS is paramount, and customer CUR data is no exception. As such we use AWS best practices to secure your data and limit access to only AWS employees on an ‘need to know’ basis for the sole purposee of using the data to provide recommendations on cost optimizations to you.

1. The process of permitting this access is opt-in and fully within your control. Access can be revoked at any time by deleting the CloudFormation stack, which will remove the IAM role that provides access.

1. The IAM role within your account (that is setup via CloudFormation) provides **read-only** access to the specific path where the CUR reports reside, no other access is given either in the S3 bucket itself or to any other S3 bucket or AWS service. The template itself is minimal in scope and its least-privilege nature can be verified easily by a CloudFormation-fluent member of your staff.

1. The CUR data, once downloaded, is stored encrypted-at-rest in a private S3 bucket in an internal AWS account which is secured by AWS internal security protocols.

1. Access to your cost data is granted to only to the AWS employees that are involved in supporting your account (your TAM's) and access is granted via a MFA protected IAM role to those individuals.

1. At any point in time your CUR data can be purged within 48 hours. Please contact your TAMs if you wish to initiate this process.
