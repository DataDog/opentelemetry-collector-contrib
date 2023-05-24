# CloudWatch Metrics Receiver

| Status                   |               |
| ------------------------ | ------------- |
| Stability                | [development] |
| Supported pipeline types | metrics       |
| Distributions            | [contrib]     |

Receives Cloudwatch metrics from [AWS Cloudwatch](https://aws.amazon.com/cloudwatch/) via the [AWS SDK for Cloudwatch Logs](https://docs.aws.amazon.com/sdk-for-go/api/service/cloudwatchlogs/)

## Getting Started

This receiver uses the [AWS SDK](https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/) as mode of authentication, which includes [Profile](https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html) and [IMDS](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html) authentication for EC2 instances.

## Configuration

### Top Level Parameters

| Parameter       | Notes      | type   | Description                                                                                                                                                                                                                                                                       |
| --------------- | ---------- | ------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `region`        | *required* | string | The AWS recognized region string  |
| `profile`       | *optional* | string | The AWS profile used to authenticate, if none is specified the default is chosen from the list of profiles  |
| `IMDSEndpoint`  | *optional* | string | The IMDS endpoint to authenticate to AWS  |                                                                                                                                 
| `poll_interval`   | `default=1m` | duration   | The duration waiting in between requests | 
| `metrics`          | *optional* | `Metrics` | Configuration for metrics ingestion of this receiver    |

### Metrics Parameters


| Parameter                | Notes        | type                   | Description                                                                                |
| ------------------------ | ------------ | ---------------------- | ------------------------------------------------------------------------------------------ |
| `named`                 | *required*   | `See Named Parameters` | Configuration for Named Metrics, by default no metrics are collected |


### Named Parameters

| Parameter                | Notes        | type                   | Description                                                                                |
| ------------------------ | ------------ | ---------------------- | ------------------------------------------------------------------------------------------ |
| `namespace`                 | *required*   | `string` | AWS Metric namespace, all AWS namespaces are prefixed with `AWS`, eg: `AWS/EC2` for EC2 metrics |
| `metric_name` | *required* | `string` | AWS metric name |
| `period` | `default=5m` | duration | Aggregation period |
| `aws_aggregation` | `default=sum` | string | type of AWS aggregation, eg: sum, min, max, average |
| `dimensions` | *optional* | `see Dimensions Parameters` | Configuration for metric dimensions |

### Dimension Parameters

| Parameter                | Notes        | type                   | Description                                                                                |
| ------------------------ | ------------ | ---------------------- | ------------------------------------------------------------------------------------------ |
| `name`                 | *required*   | `string` | Dimensions name, can't start with a colon |
| `value` | *required* | `string` | Dimension value |


#### Named Example

```yaml
awscloudwatchmetrics:
  region: us-east-1
  poll_interval: 1m
  metrics:
    named:
      - namespace: "AWS/EC2"
        metric_name: "CPUUtilization"
        period: "5m"
        aws_aggregation: "Sum"
        dimensions:
          - Name: "InstanceId"
            Value: "i-1234567890abcdef0"
      - namespace: "AWS/S3"
        metric_name: "BucketSizeBytes"
        period: "5m"
        aws_aggregation: "p99"
        dimensions:
          - Name: "BucketName"
            Value: "OpenTelemetry"
          - Name: "StorageType"
            Value: "StandardStorage"
```

## Sample Configs

```yaml
receivers:
  awscloudwatchmetrics:
    region: eu-west-1
    poll_interval: 10m
    metrics:
      named:
        - namespace: "AWS/EC2"
          metric_name: "CPUUtilization"
          period: "5m"
          aws_aggregation: "Sum"
          dimensions:
            - Name: "InstanceId"
              Value: "i-035e091c31292427a"

processors:

exporters:
  logging:
    verbosity: detailed

service:
  pipelines:
    metrics:
      receivers: [awscloudwatchmetrics]
      processors: []
      exporters: [logging]
```

## AWS Costs

This receiver uses the [GetMetricData](https://docs.aws.amazon.com/AmazonCloudWatch/latest/APIReference/API_GetMetricData.html) API call, this call is *not* in the AWS free tier. Please refer to [Amazon's pricing](https://aws.amazon.com/cloudwatch/pricing/) for further information about expected costs.


[alpha]:https://github.com/open-telemetry/opentelemetry-collector#alpha
[contrib]:https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol-contrib
[Issue]:https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/15667

## Troubleshooting / Debugging

## My metrics are intermittent / not receing any metrics

Try a bigger `poll_interval`. CloudWatch returns no data if the period of the metric, by default for AWS supplied metrics, it's 300 seconds (5 minutes). Try out a period of 600 seconds and a poll interval of 600 seconds.

## Help, I'm getting IAM permission denied

Make sure your IAM role/user has the required permissions:

```yaml
"cloudwatch:GetMetricData",
"cloudwatch:GetMetricStatistics",
"cloudwatch:ListMetrics"
```

The following IAM permissions are required for transit gateways to work:

```
"ec2:DescribeTags",
"ec2:DescribeInstances",
"ec2:DescribeRegions",
"ec2:DescribeTransitGateway*"
```

