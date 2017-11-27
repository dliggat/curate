package main

import (
	"fmt"
	"errors"
	"github.com/andyfase/CURdashboard/go/curconvert"
	"log"
	"encoding/json"
	"time"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

type Message struct {
	Name string
	Value int64
}

func gosqs() (Message, error) {
	var m Message
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
 	svc := sqs.New(sess)
	qURL := "https://sqs.us-west-2.amazonaws.com/824550351281/cursqs-development-Queue-VF7QJ75CC880"

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

		fmt.Println("Message Deleted", resultDelete)
		var m Message
		if err := json.Unmarshal([]byte(*result.Messages[0].Body), &m); err != nil {
			return m, err
		}
		return m, nil
	}

}


func main() {

	m, err := gosqs()
	if err != nil {
		fmt.Println(err)
		fmt.Println("Nothing to do. Exiting.")
		return
	}

	fmt.Printf("Result: %+v", m)

	var sourceBucket, destBucket, destPath, reportPath, reportName string //inputDate, sourceRoleArn, sourceExternalID, destRoleArn, destExternalID string
	sourceBucket = "dliggat-billing"
	destBucket = "dliggat-gocur"
	reportPath = "foo/bar/newreport"
	reportName = "newreport"

	var start time.Time
	start = time.Now()
	end := start.AddDate(0, 1, 0)
	curDate := start.Format("200601") + "01-" + end.Format("200601") + "01"
	manifest := reportPath + "/" + curDate + "/" + reportName + "-Manifest.json"

	fmt.Println(manifest)
	destPath = "parquet-cur/" + start.Format("200601")

	cc := curconvert.NewCurConvert(sourceBucket, manifest, destBucket, destPath)
	if err := cc.ConvertCur(); err != nil {
		log.Fatalln(err)
	}
	fmt.Println("CUR conversion completed and available at s3://" + destBucket + "/" + destPath + "/")
}
