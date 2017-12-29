---
title: "Is their any assosiated costs?"
date: 2017-12-27T14:11:37-08:00
anchor: "cost"
weight: 2
---

The [AWS Cost and Usage Reports](http://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/billing-reports-costusage.html) are free of charge, however standard [S3 pricing](https://aws.amazon.com/s3/pricing/) applies to the storage of the reports.

By providing access to your CUR reports there may be a small S3 cost for the automated fetching of the S3 CUR report files. **However** these costs can be avoided if your CUR bucket is setup in `us-east-1`.

* IF your CUR S3 bucket is in `us-east-1` there is no additional cost. This is because the infrastructure that downloads the CUR files from your S3 bucket is setup in `us-east-1`. Transfers from S3 to any service(s) within the same region are free - hence this traffic will incur no costs.

* If your CUR S3 bucket is in any other region there will be a small S3 cost for data transfer. The actual cost incurred will depend on the size of the CUR files, these will be larger for customers with larger overall AWS deployments. For small deployments the cost is estimated at less than $0.50 per month. For larger deployments the cost is estimated at less than $5 per month

_**We recommend your S3 bucket hosting your CUR reports reside in `us-east-1` so that any costs can be avoided.**_

