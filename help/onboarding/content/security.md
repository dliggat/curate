---
title: "What about security?"
date: 2017-12-27T14:11:41-08:00
anchor: "security"
weight: 3
---

AWS takes the security of your cost data seriously. As such we use AWS best practices to secure your data and limit access to only AWS employees that can use the data to provide recommendations on cost optimizations to you. 

1. The process of permitting this access is opt-in and fully within your control. Access can be revoked at any time by deleting the Cloudformation stack, which will remove the IAM role that provides access.

1. The IAM role within your account (that is setup via Cloudformation) provides **read-only** access to the specific path where the CUR reports reside, no other access is given either in the S3 bucket itself or to any other S3 bucket or AWS service.

1. The CUR data once downloaded is stored encrypted-at-rest in a private S3 bucket in an internal AWS account which is secured by AWS internal security protocols.

1. Access to your cost data is granted to only the AWS employees that are associated to supporting your account (your TAM's) and access is granted via a MFA protected IAM role to those individuals.

1. At any point in time your costs data can be deleted within 48 hours, please contact your TAM's if you wish to start this process. 