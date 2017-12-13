package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/andyfase/CURdashboard/go/curconvert"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/athena"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/jcxplorer/cwlogger"
)

func getInstanceMetadata(sess *session.Session) map[string]interface{} {
	c := &http.Client{
		Timeout: 100 * time.Millisecond,
	}
	resp, err := c.Get("http://169.254.169.254/latest/dynamic/instance-identity/document")
	var m map[string]interface{}
	if err == nil {
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			_ = json.Unmarshal(body, &m)
		}
	}
	// if we havent obtained instance meta-data fetch account from STS - likely were not on EC2
	_, ok := m["region"].(string)
	if !ok {
		svc := sts.New(sess)
		result, err := svc.GetCallerIdentity(&sts.GetCallerIdentityInput{})
		if err == nil {
			m = make(map[string]interface{})
			m["accountId"] = *result.Account
			m["region"] = *sess.Config.Region
		}
	}
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

/*
Function takes SQL to send to Athena converts into JSON to send to Athena HTTP proxy and then sends it.
Then recieves responses in JSON which is converted back into a struct and returned
*/
func sendQuery(svc *athena.Athena, db string, sql string, account string, region string) error {
	var s athena.StartQueryExecutionInput
	s.SetQueryString(sql)

	var q athena.QueryExecutionContext
	q.SetDatabase(db)
	s.SetQueryExecutionContext(&q)

	var r athena.ResultConfiguration
	r.SetOutputLocation("s3://aws-athena-query-results-" + account + "-" + region + "/")
	s.SetResultConfiguration(&r)

	result, err := svc.StartQueryExecution(&s)
	if err != nil {
		return errors.New("Error Querying Athena, StartQueryExecution: " + err.Error())
	}

	var qri athena.GetQueryExecutionInput
	qri.SetQueryExecutionId(*result.QueryExecutionId)

	var qrop *athena.GetQueryExecutionOutput
	duration := time.Duration(2) * time.Second

	for {
		qrop, err = svc.GetQueryExecution(&qri)
		if err != nil {
			return errors.New("Error Querying Athena, GetQueryExecution: " + err.Error())
		}
		if *qrop.QueryExecution.Status.State != "RUNNING" {
			break
		}
		time.Sleep(duration)
	}

	if *qrop.QueryExecution.Status.State != "SUCCEEDED" {
		return errors.New("Error Querying Athena, completion state is NOT SUCCEEDED, state is: " + *qrop.QueryExecution.Status.State)
	}
	return nil
}

func createAthenaTable(sess *session.Session, tableprefix string, database string, columns []curconvert.CurColumn, s3path string, meta map[string]interface{}) error {
	if len(database) < 1 {
		database = "cur"
	}

	svcAthena := athena.New(sess)

	sql := "CREATE DATABASE IF NOT EXISTS `" + database + "`"
	if err := sendQuery(svcAthena, "default", sql, meta["accountId"].(string), meta["region"].(string)); err != nil {
		return errors.New("Could not create Athena Database, error: " + err.Error())
	}

	var cols string
	for col := range columns {
		cols += "`" + columns[col].Name + "` " + columns[col].Type + ",\n"
	}
	cols = cols[:strings.LastIndex(cols, ",")]
	start := time.Now()
	table := tableprefix + "_" + start.Format("200601")

	sql = "CREATE EXTERNAL TABLE IF NOT EXISTS `" + table + "` (" + cols + ") STORED AS PARQUET LOCATION '" + s3path + "'"
	if err := sendQuery(svcAthena, database, sql, meta["accountId"].(string), meta["region"].(string)); err != nil {
		return errors.New("Could not create Athena Database, error: " + err.Error())
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
	CurDatabase           string `json:"cur_database"`
}

func processCUR(m Message, topLevelDestPath string) ([]curconvert.CurColumn, string, error) {
	if len(m.SourceBucket) < 1 {
		return nil, "", errors.New("Must supply a source bucket")
	}
	if len(m.CurReportDescriptor) < 1 {
		return nil, "", errors.New("Must supply a report descriptor")
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

	if err := cc.ConvertCur(); err != nil {
		return nil, "", errors.New("Could not convert CUR: " + err.Error())
	}

	cols, err := cc.GetCURColumns()
	if err != nil {
		return nil, "", errors.New("Could not obtain CUR columns: " + err.Error())
	}

	return cols, "s3://" + m.DestinationBucket + "/" + destPath + "/", nil
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

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	meta := getInstanceMetadata(sess)

	// Check if running on EC2
	_, ec2 := meta["instanceId"].(string)
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
			for _, message := range resp.Messages {
				var m Message
				if err := json.Unmarshal([]byte(*message.Body), &m); err != nil {
					doLog(logger, "Failed to decode message job: "+err.Error())
				} else {
					doLog(logger, "Starting processing of job, arn: "+m.CurReportDescriptor+" on bucket: "+m.SourceBucket)
					columns, s3path, err := processCUR(m, topLevelDestPath)
					if err != nil {
						doLog(logger, "Failed to process CUR conversion, error: "+err.Error())
					} else {
						if err = createAthenaTable(sess, m.CurReportDescriptor, m.CurDatabase, columns, s3path, meta); err != nil {
							doLog(logger, "Falied to create/update Athena tables, error: "+err.Error())
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
								doLog(logger, "Completed processing of job, arn: "+m.CurReportDescriptor+" on bucket: "+m.SourceBucket)
							}
						}
					}
				}
			}
		}
	}
}
