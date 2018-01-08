---
title: "Step by Step setup"
date: 2017-12-27T12:14:46-08:00
anchor: "setup"
weight: 4
---

The following steps assume appropriate access to the AWS console. Permission is needed to view the AWS billing console and to spin up a new CloudFormation stack.


1. If not already setup, [enable AWS Cost and Usage reports](http://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/billing-reports-gettingstarted-turnonreports.html)

1. **<a href="https://console.aws.amazon.com/cloudformation/home?#cstack=sn~aws-cur-tam-trust|turl~https://s3.amazonaws.com/aws-tam-cur-setup/customer-bucket-trust.yaml" target="_blank">Click here to launch the S3 CUR Trust Stack</a>** which starts the process of creating a new CloudFormation stack
(opens a new window)

1. Click **Next** to move onto the required stack parameters.
![CUR Report Screenshot](/cfn-cur-report-screenshot-2.png)

1. **<a href="https://console.aws.amazon.com/billing/home?#/reports" target="_blank">Click here to view the AWS Cost and Usage Reports</a>** within the AWS Billing console.
(opens a new window)

1. Copy the configured *Report Name* and *Report Path* from the billing report in the AWS Billing console and paste the values into the `BucketName` and `ReportPath` fields within the CloudFormation stack parameters. The required values from the billing report are highlighted in the screen-shot below. Then click **Next**
![CUR Report Screenshot](/cfn-cur-report-screenshot.png)
![CUR Report Screenshot](/cfn-cur-report-screenshot-3.png)

1. Click **Next** to move onto the review of the CloudFormation stack creation

1. Click the check-box labeled *'I acknowledge that AWS CloudFormation might create IAM resources.'* and finally click **Create**
![CUR Report Screenshot](/cfn-cur-report-screenshot-4.png)

1. Press the refresh button within the CloudFormation console until the stack 'aws-cur-tam-trust' has a status of **'CREATE_COMPLETE'**

1. Select the CloudFormation stack and under the 'Outputs' tab copy all 4 keys with their values and send them to your TAM. These keys should be (**SigninUrl**, **CrossAccountRoleArn**, **CrossAccountRoleName** and **ExternalID**)

This completes the setup. Thank you!
