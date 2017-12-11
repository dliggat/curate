package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/andyfase/CURdashboard/go/curconvert"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/jcxplorer/cwlogger"
)

func getInstanceMetadata() map[string]interface{} {
	c := &http.Client{
		Timeout: 100 * time.Millisecond,
	}
	resp, err := c.Get("http://169.254.169.254/latest/dynamic/instance-identity/document")
	var m map[string]interface{}
	if err != nil {
		return m
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	// Unmrshall json, if it errors ignore and return empty map
	_ = json.Unmarshal(body, &m)
	return m
}

func getParams(queueUrl *string, topLevelDestPath *string) error {
	flag.StringVar(queueUrl, "sqsqueue", "", "SQS URL for processing")
	flag.StringVar(topLevelDestPath, "destpathprefix", "parquet-cur", "Top level destination path")
	flag.Parse()

	if len(*queueUrl) < 1 {
		return errors.New("Provide valid SQS Queue URL")
	}

	if len(*topLevelDestPath) < 1 {
		return errors.New("Provide valid Destination Path")
	}

	return nil
}

type Message struct {
	CurReportDescriptor   string `json:"cur_report_descriptor"`
	SourceBucket          string `json:"source_bucket"`
	DestinationBucket     string `json:"destination_bucket"`
	ReportPath            string `json:"report_path"`
	ReportName            string `json:"report_name"`
	SourceRoleArn         string `json:"source_role_arn"`
	SourceExternalId      string `json:"source_external_id"`
	DestinationRoleArn    string `json:"destination_role_arn"`
	DestinationExternalId string `json:"destination_external_id"`
}

func processCUR(m Message, topLevelDestPath string) error {
	if len(m.SourceBucket) < 1 {
		return errors.New("Must supply a source bucket")
	}
	if len(m.CurReportDescriptor) < 1 {
		return errors.New("Must supply a report descriptor")
	}

	start := time.Now()
	end := start.AddDate(0, 1, 0)
	curDate := start.Format("200601") + "01-" + end.Format("200601") + "01"
	manifest := m.ReportPath + "/" + curDate + "/" + m.ReportName + "-Manifest.json"
	destPath := topLevelDestPath + "/" + m.CurReportDescriptor + "/" + start.Format("200601")

	cc := curconvert.NewCurConvert(m.SourceBucket, manifest, m.DestinationBucket, destPath)
	if len(m.SourceRoleArn) > 1 {
		cc.SetSourceRole(m.SourceRoleArn, m.SourceExternalId)
	}
	if len(m.DestinationRoleArn) > 1 {
		cc.SetSourceRole(m.DestinationRoleArn, m.DestinationExternalId)
	}
	return cc.ConvertCur()
}

func doLog(logger *cwlogger.Logger, m string) {
	if logger != nil {
		logger.Log(time.Now(), m)
	}
	log.Println(m)
}

func main() {
	// Input parameters
	var queueUrl, topLevelDestPath string
	if err := getParams(&queueUrl, &topLevelDestPath); err != nil {
		log.Fatalln(err)
	}

	// Grab instance meta-data
	meta := getInstanceMetadata()

	// Check if running on EC2 or local
	_, ec2 := meta["region"].(string)

	// AWS sess
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	var logger *cwlogger.Logger
	if ec2 { // Init Cloudwatch Logger class if were running on EC2
		logger, err := cwlogger.New(&cwlogger.Config{
			LogGroupName: "CURprocessor",
			Client:       cloudwatchlogs.New(sess),
		})
		if err != nil {
			log.Fatal("Could not initalize Cloudwatch logger: " + err.Error())
		}
		defer logger.Close()
		logger.Log(time.Now(), "CURprocessor running on "+meta["instanceId"].(string)+" in "+meta["availabilityZone"].(string))
	}

	// create sqs handler
	svc := sqs.New(sess)

	// params for SQS Message Input. 20 second long poll, single message at a time, and hold message for 30mins for processing.
	params := &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queueUrl),
		AttributeNames:      []*string{aws.String(".*")},
		MaxNumberOfMessages: aws.Int64(1),
		VisibilityTimeout:   aws.Int64(1800),
		WaitTimeSeconds:     aws.Int64(20),
	}

	// loop for messages
	for true {
		resp, err := svc.ReceiveMessage(params)
		if err != nil {
			doLog(logger, err.Error())
		} else {
			for _, message := range resp.Messages { // loop through messages in array
				// marshall JSON Body of message
				var m Message
				if err := json.Unmarshal([]byte(*message.Body), &m); err != nil {
					doLog(logger, "Failed to decode message job: "+err.Error())
				} else {
					// Do CUR conversion
					doLog(logger, "Starting processing of job, arn: "+m.SourceRoleArn+" on bucket: "+m.SourceBucket)
					if err = processCUR(m, topLevelDestPath); err != nil {
						doLog(logger, "Failed to process CUR conversion, error: "+err.Error())
					} else {
						// send back success of processing messages
						paramsDelete := &sqs.DeleteMessageInput{
							QueueUrl:      aws.String(queueUrl),
							ReceiptHandle: aws.String(*message.ReceiptHandle),
						}
						_, err = svc.DeleteMessage(paramsDelete)
						if err != nil {
							doLog(logger, "Failed to delete SQS message from queue, error: "+err.Error())
						} else {
							doLog(logger, "Completed processing of job, arn: "+m.SourceRoleArn+" on bucket: "+m.SourceBucket)
						}
					}
				}
			}
		}
	}
}
