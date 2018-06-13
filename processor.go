package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/andyfase/CURdashboard/go/curconvert"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/athena"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/glue"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/jcxplorer/cwlogger"
)

type instanceProtection struct {
	sess       *session.Session
	instanceID string
	asgName    string
	state      bool
	s          chan bool
}

func (ip *instanceProtection) set(state bool) error {
	ip.s <- true
	var lastError error
	if ip.state != state {
		svc := autoscaling.New(ip.sess)
		input := &autoscaling.SetInstanceProtectionInput{
			AutoScalingGroupName: aws.String(ip.asgName),
			InstanceIds: []*string{
				aws.String(ip.instanceID),
			},
			ProtectedFromScaleIn: aws.Bool(state),
		}

		for i := 1; i < 6; i++ {
			if lastError != nil {
				time.Sleep(time.Second * time.Duration(5*i))
			}
			_, lastError = svc.SetInstanceProtection(input)
			if lastError == nil {
				ip.state = state
				break
			}
		}
	}
	<-ip.s
	return lastError
}

func getASGForInstance(sess *session.Session, instanceID string) (string, error) {
	svc := autoscaling.New(sess)
	for i := 1; i < 6; i++ {
		resp, err := svc.DescribeAutoScalingInstances(
			&autoscaling.DescribeAutoScalingInstancesInput{
				InstanceIds: []*string{
					aws.String(instanceID),
				},
				MaxRecords: aws.Int64(1),
			})
		if err == nil && len(resp.AutoScalingInstances) > 0 && len(*resp.AutoScalingInstances[0].AutoScalingGroupName) > 0 {
			return *resp.AutoScalingInstances[0].AutoScalingGroupName, nil
		}
		time.Sleep(time.Second * time.Duration(5*i))
	}
	return "", fmt.Errorf("Failed to fetch ASG Name for %s", instanceID)
}

func waitForASGStatus(sess *session.Session, instanceID string, state string) error {
	svc := autoscaling.New(sess)
	for i := 1; i < 6; i++ {
		resp, err := svc.DescribeAutoScalingInstances(
			&autoscaling.DescribeAutoScalingInstancesInput{
				InstanceIds: []*string{
					aws.String(instanceID),
				},
				MaxRecords: aws.Int64(1),
			})
		if err == nil && len(resp.AutoScalingInstances) > 0 {
			if *resp.AutoScalingInstances[0].LifecycleState == state {
				return nil
			}
		}
		time.Sleep(time.Second * time.Duration(5*i))
	}
	return fmt.Errorf("timeout waiting for instance %s to reach %s state", instanceID, state)
}

func getInstanceMetadata(sess *session.Session) map[string]interface{} {
	c := &http.Client{
		Timeout: 100 * time.Millisecond,
	}
	resp, err := c.Get("http://169.254.169.254/latest/dynamic/instance-identity/document")
	var m map[string]interface{}
	if err == nil {
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err == nil {
			err = json.Unmarshal(body, &m)
			if err != nil {
				log.Fatalln("Could not parse MetaData, erorr: " + err.Error())
			}
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

func getParams(queueUrl *string, topLevelDestPath *string, scratchDir *string, healthPort *string) error {
	flag.StringVar(queueUrl, "sqsqueue", "", "SQS URL for processing")
	flag.StringVar(topLevelDestPath, "destpathprefix", "parquet-cur", "Top level destination path")
	flag.StringVar(healthPort, "healthport", "80", "Health port to listen on")
	flag.StringVar(scratchDir, "tmp", "/tmp", "Directory to store temporary files using conversion")
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
	r.SetOutputLocation("s3://aws-athena-query-results-" + account + "-" + region + "/feedprocessor/")
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

func createUpdateAthenaTable(sess *session.Session, m Message, columns []curconvert.CurColumn, s3path string, meta map[string]interface{}, curDate string) error {
	svcGlue := glue.New(sess)
	table := m.CurReportDescriptor + "_" + curDate

	// if Table exists and if so update it - otherwise create it
	resp, err := svcGlue.GetTable(&glue.GetTableInput{
		DatabaseName: aws.String(m.CurDatabase),
		Name:         aws.String(table)})

	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == glue.ErrCodeEntityNotFoundException {
				// Table doesnt exist create it
				createAthenaTable(sess, m, columns, s3path, meta, curDate)
			} else {
				return errors.New("Failed to check existing table, error: " + awsErr.Message())
			}
		} else {
			return errors.New("Failed to check existing table, error: " + err.Error())
		}
	}

	var cols []*glue.Column
	for col := range columns {
		col := &glue.Column{
			Name: aws.String(columns[col].Name),
			Type: aws.String(columns[col].Type)}
		cols = append(cols, col)
	}

	// update column info in existing table
	updateTableInput := &glue.UpdateTableInput{
		DatabaseName: aws.String(m.CurDatabase),
		TableInput: &glue.TableInput{
			Name:              aws.String(table),
			StorageDescriptor: &glue.StorageDescriptor{Columns: cols}}}

	if _, err := svcGlue.UpdateTable(updateTableInput); err != nil {
		return errors.New("Error updating table column info, error: " + err.Error())
	}

	return nil
}

func createAthenaTable(sess *session.Session, m Message, columns []curconvert.CurColumn, s3path string, meta map[string]interface{}, curDate string) error {
	svcAthena := athena.New(sess)

	sql := "CREATE DATABASE IF NOT EXISTS `" + m.CurDatabase + "`"
	if err := sendQuery(svcAthena, "default", sql, meta["accountId"].(string), meta["region"].(string)); err != nil {
		return errors.New("Could not create Athena Database, error: " + err.Error())
	}

	var cols string
	for col := range columns {
		cols += "`" + columns[col].Name + "` " + columns[col].Type + ",\n"
	}
	cols = cols[:strings.LastIndex(cols, ",")]
	table := m.CurReportDescriptor + "_" + curDate

	sql = "CREATE EXTERNAL TABLE IF NOT EXISTS `" + table + "` (" + cols + ") STORED AS PARQUET LOCATION '" + s3path + "'"
	if len(m.DestinationKMSKeyArn) > 0 {
		sql += " TBLPROPERTIES ('has_encrypted_data'='true')"
	}

	if err := sendQuery(svcAthena, m.CurDatabase, sql, meta["accountId"].(string), meta["region"].(string)); err != nil {
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
	DestinationKMSKeyArn  string `json:"destination_kms_key_arn"`
	CurDatabase           string `json:"cur_database"`
	Date                  string `json:date`
}

func processCUR(m Message, topLevelDestPath string, scratchDir string, logger *cwlogger.Logger) ([]curconvert.CurColumn, string, string, error) {
	if len(m.SourceBucket) < 1 {
		return nil, "", "", errors.New("Must supply a source bucket")
	}
	if len(m.CurReportDescriptor) < 1 {
		return nil, "", "", errors.New("Must supply a report descriptor")
	}

	var t1 time.Time
	var err error
	if len(m.Date) > 0 {
		doLog(logger, "Overriding date for CUR conversion to "+m.Date)
		t1, err = time.Parse("20060102", m.Date)
		if err != nil {
			return nil, "", "", errors.New("Could not parse given date overide: " + m.Date + ", " + err.Error())
		}
	} else {
		t1 = time.Now()
	}

	t1First := time.Date(t1.Year(), t1.Month(), 1, 0, 0, 0, 0, time.Local)
	t2 := t1First.AddDate(0, 1, 0)
	t2First := time.Date(t2.Year(), t2.Month(), 1, 0, 0, 0, 0, time.Local)

	curDate := fmt.Sprintf("%d%02d01-%d%02d01", t1First.Year(), t1First.Month(), t2First.Year(), t2First.Month())
	manifest := m.ReportPath + "/" + curDate + "/" + m.ReportName + "-Manifest.json"

	destPathDate := fmt.Sprintf("%d%02d", t1First.Year(), t1First.Month())
	destPath := topLevelDestPath + "/" + m.CurDatabase + "/" + m.CurReportDescriptor + "/" + destPathDate

	cc := curconvert.NewCurConvert(m.SourceBucket, manifest, m.DestinationBucket, destPath)
	if len(m.SourceRoleArn) > 1 {
		cc.SetSourceRole(m.SourceRoleArn, m.SourceExternalId)
	}
	if len(m.DestinationRoleArn) > 1 {
		cc.SetSourceRole(m.DestinationRoleArn, m.DestinationExternalId)
	}
	if len(m.DestinationKMSKeyArn) > 1 {
		cc.SetDestKMSKey(m.DestinationKMSKeyArn)
	}
	if len(scratchDir) > 0 {
		cc.SetTmpLocation(scratchDir)
	}

	// Check current months manifest exists
	if err := cc.CheckCURExists(); err != nil {
		if err.(awserr.Error).Code() != s3.ErrCodeNoSuchKey {
			return nil, "", "", errors.New("Error fetching CUR Manifest: " + err.Error())
		}
		if t1.Day() > 3 {
			return nil, "", "", errors.New("Error fetching CUR Manifest, NoSuchKey and too delayed: " + err.Error())
		}
		// Regress to processing last months CUR. Error is ErrCodeNoSuchKey and still early in the month
		doLog(logger, "Reseting to previous months CUR for "+m.CurReportDescriptor)
		t1First = t1First.AddDate(0, 0, -1)
		t2First = t2First.AddDate(0, 0, -1)
		curDate = fmt.Sprintf("%d%02d01-%d%02d01", t1First.Year(), t1First.Month(), t2First.Year(), t2First.Month())
		manifest = m.ReportPath + "/" + curDate + "/" + m.ReportName + "-Manifest.json"
		cc.SetSourceManifest(manifest)

		destPathDate = fmt.Sprintf("%d%02d", t1First.Year(), t1First.Month())
		destPath = topLevelDestPath + "/" + m.CurDatabase + "/" + m.CurReportDescriptor + "/" + destPathDate
		cc.SetDestPath(destPath)
	}

	if err := cc.ConvertCur(); err != nil {
		return nil, "", "", errors.New("Could not convert CUR: " + err.Error())
	}

	cols, err := cc.GetCURColumns()
	if err != nil {
		return nil, "", "", errors.New("Could not obtain CUR columns: " + err.Error())
	}

	return cols, "s3://" + m.DestinationBucket + "/" + destPath + "/", destPathDate, nil
}

func doLog(logger *cwlogger.Logger, m string) {
	if logger != nil {
		logger.Log(time.Now(), m)
	}
	log.Println(m)
}

func main() {
	// Input parameters
	var queueUrl, topLevelDestPath, scratchDir, healthPort string
	if err := getParams(&queueUrl, &topLevelDestPath, &scratchDir, &healthPort); err != nil {
		log.Fatalln(err)
	}

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	meta := getInstanceMetadata(sess)

	// re-init session now we have the region we are in
	sess = sess.Copy(&aws.Config{Region: aws.String(meta["region"].(string))})

	// Check if running on EC2
	_, ec2 := meta["instanceId"].(string)
	var asgName string
	var logger *cwlogger.Logger
	if ec2 { // Init Cloudwatch Logger class if were running on EC2
		var err error
		logger, err = cwlogger.New(&cwlogger.Config{
			LogGroupName: "curate",
			Client:       cloudwatchlogs.New(sess),
		})
		if err != nil {
			log.Fatal("Could not initalize Cloudwatch logger: " + err.Error())
		}
		defer logger.Close()
		doLog(logger, "curate runnning on "+meta["instanceId"].(string)+" in "+meta["availabilityZone"].(string))

		asgName, err = getASGForInstance(sess, meta["instanceId"].(string))
		if err != nil {
			doLog(logger, "couldnt find ASG for "+meta["instanceId"].(string)+"in "+meta["availabilityZone"].(string)+" Error: "+err.Error())
		} else {
			doLog(logger, "curate running on "+meta["instanceId"].(string)+" witin "+asgName+" in "+meta["availabilityZone"].(string))
		}
	}

	// Setup process HTTP health check
	go func() {
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "healthy")
		})
		doLog(logger, "Listening on Port "+healthPort+" for health check")
		log.Fatal(http.ListenAndServe(":"+healthPort, nil))
	}()

	// create sqs handler
	svc := sqs.New(sess)

	// params for SQS Message Input. 20 second long poll, single message at a time
	params := &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queueUrl),
		AttributeNames:      []*string{aws.String(".*")},
		MaxNumberOfMessages: aws.Int64(1),
		WaitTimeSeconds:     aws.Int64(20),
	}

	// Setup InstanceProtection struct
	var ip *instanceProtection
	if ec2 && len(asgName) > 0 {
		ip = &instanceProtection{
			sess:       sess,
			instanceID: meta["instanceId"].(string),
			asgName:    asgName,
			state:      false,
			s:          make(chan bool, 1),
		}
		// wait for InService State
		err := waitForASGStatus(sess, meta["instanceId"].(string), "InService")
		if err != nil {
			log.Fatal("Timeout waiting for instance to move into service " + err.Error())
		}
	}

	// loop for messages
	for true {
		resp, err := svc.ReceiveMessage(params)
		if err != nil {
			doLog(logger, err.Error())
		} else {
			if ip != nil {
				go func() {
					err := ip.set(true)
					if err != nil {
						doLog(logger, "Failed to set instance protection for instance "+meta["instanceId"].(string)+" on ASG "+asgName+" error: "+err.Error())
					}
				}()
			}
			for _, message := range resp.Messages {
				var m Message
				if err := json.Unmarshal([]byte(*message.Body), &m); err != nil {
					doLog(logger, "Failed to decode message job: "+err.Error())
				} else {
					doLog(logger, "Starting processing of job, report: "+m.CurReportDescriptor+", bucket: "+m.SourceBucket)

					if len(m.CurDatabase) < 1 {
						m.CurDatabase = "cur"
					}
					columns, s3path, curDate, err := processCUR(m, topLevelDestPath, scratchDir, logger)
					if err != nil {
						doLog(logger, "Failed to process CUR conversion for report: "+m.CurReportDescriptor+", error: "+err.Error())
					} else {
						if err = createUpdateAthenaTable(sess, m, columns, s3path, meta, curDate); err != nil {
							doLog(logger, "Falied to create/update Athena tables, report: "+m.CurReportDescriptor+" error: "+err.Error())
						} else {
							// send back success of processing messages
							paramsDelete := &sqs.DeleteMessageInput{
								QueueUrl:      aws.String(queueUrl),
								ReceiptHandle: aws.String(*message.ReceiptHandle),
							}
							_, err = svc.DeleteMessage(paramsDelete)
							if err != nil {
								doLog(logger, "Failed to delete SQS message from queue, report: "+m.CurReportDescriptor+" error: "+err.Error())
							} else {
								doLog(logger, "Completed processing of job, report: "+m.CurReportDescriptor+", bucket: "+m.SourceBucket)
							}
						}
					}
				}
			}
			if ip != nil {
				go func() {
					err := ip.set(false)
					if err != nil {
						doLog(logger, "Failed to unset instance protection for instance "+meta["instanceId"].(string)+" on ASG "+asgName+" error: "+err.Error())
					}
				}()
			}
		}
	}
}
