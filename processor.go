package main

import (
	"fmt"
	"flag"
	"errors"
	"github.com/andyfase/CURdashboard/go/curconvert"
	"log"
	"encoding/json"
	"time"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)


func gosqs(queueUrl string) (Message, error) {
	var m Message
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
 	svc := sqs.New(sess)
	qURL := queueUrl

	result, err := svc.ReceiveMessage(&sqs.ReceiveMessageInput{
	AttributeNames: []*string{
		aws.String(sqs.MessageSystemAttributeNameSentTimestamp),
	},
	MessageAttributeNames: []*string{
		aws.String(sqs.QueueAttributeNameAll),
	},
	QueueUrl:            &qURL,
	MaxNumberOfMessages: aws.Int64(1),
	VisibilityTimeout:   aws.Int64(36000),  // 10 hours
	WaitTimeSeconds:     aws.Int64(0),
	})

	if err != nil {
		return m, err
	}

	if len(result.Messages) == 0 {
		return m, errors.New("Received no messages")
	} else {
		resultDelete, err := svc.DeleteMessage(&sqs.DeleteMessageInput{
			QueueUrl:      &qURL,
			ReceiptHandle: result.Messages[0].ReceiptHandle,
		})
		if err != nil {
			return m, err
		}
		fmt.Println(*result.Messages[0].Body)

		fmt.Println("Message Deleted", resultDelete)
		var m Message
		if err := json.Unmarshal([]byte(*result.Messages[0].Body), &m); err != nil {
			return m, err
		}
		return m, nil
	}

}

func getParams(queueUrl *string, topLevelDestPath *string) error {
	flag.StringVar(queueUrl, "sqsqueue", "", "SQS URL for processing")
	flag.StringVar(topLevelDestPath, "destpathprefix", "parquet-cur", "Top level destination path")
	flag.Parse()
	return nil
}

type Message struct {
	CurReportDescriptor string    `json:"cur_report_descriptor"`
	SourceBucket string           `json:"source_bucket"`
	DestinationBucket string      `json:"destination_bucket"`
        ReportPath string             `json:"report_path"`
        ReportName string             `json:"report_name"`
	SourceRoleArn string          `json:"source_role_arn"`
	SourceExternalId string       `json:"source_external_id"`
	DestinationRoleArn string     `json:"destination_role_arn"`
	DestinationExternalId string  `json:"destination_external_id"`
}


func main() {
	var queueUrl, topLevelDestPath string
	if err := getParams(&queueUrl, &topLevelDestPath); err != nil {
		fmt.Println(err)
		return
	}


	m, err := gosqs(queueUrl)
	if err != nil {
		fmt.Println(err)
		fmt.Println("Nothing to do. Exiting.")
		return
	}

	fmt.Printf("Result: %+v", m)
	if len(m.SourceBucket) < 1 {
		log.Fatalln("Must supply a source bucket")
	}
	if len(m.CurReportDescriptor) < 1 {
		log.Fatalln("Must supply a report descriptor")
	}

	var start time.Time
	start = time.Now()
	end := start.AddDate(0, 1, 0)
	curDate := start.Format("200601") + "01-" + end.Format("200601") + "01"
	manifest := m.ReportPath + "/" + curDate + "/" + m.ReportName + "-Manifest.json"

	fmt.Println(manifest)
	destPath := topLevelDestPath + "/" + m.CurReportDescriptor + "/" + start.Format("200601")

	cc := curconvert.NewCurConvert(m.SourceBucket, manifest, m.DestinationBucket, destPath)

	if len(m.SourceRoleArn) > 1 {
		cc.SetSourceRole(m.SourceRoleArn, m.SourceExternalId)
	}
	if len(m.DestinationRoleArn) > 1 {
		cc.SetSourceRole(m.DestinationRoleArn, m.DestinationExternalId)
	}

	if err := cc.ConvertCur(); err != nil {
		log.Fatalln(err)
	}
	fmt.Println("CUR conversion completed and available at s3://" + m.DestinationBucket + "/" + destPath + "/")
}
