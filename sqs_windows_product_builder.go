package main

import "C"
import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/sqs"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

var TempLocationToStoreBinaries = "/tmp" // C:/Temp for windows
type ProductBuild struct {
	Binaries string `json:"binaries"`
	Destination string `json:"destination"`
	GitRevision string `json:"revision"`
	Branch string `json:"branch"`
	Commit string `json:"commit"`
	Product string `json:"product"`
	User string `json:"user"`
	Platform string `json:"platform"`
	Bucket string
	SourceWithOutBucketName string
	DestinationWithOutBucketName string
 }

const defaultFailedCode = 1

func RunCommand(name string, args ...string) (stdout string, stderr string, exitCode int) {
	log.Println("run command:", name, args)
	var outbuf, errbuf bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &outbuf
	cmd.Stderr = &errbuf

	err := cmd.Run()
	stdout = outbuf.String()
	stderr = errbuf.String()

	if err != nil {
		// try to get the exit code
		if exitError, ok := err.(*exec.ExitError); ok {
			ws := exitError.Sys().(syscall.WaitStatus)
			exitCode = ws.ExitStatus()
		} else {
			// This will happen (in OSX) if `name` is not available in $PATH,
			// in this situation, exit code could not be get, and stderr will be
			// empty string very likely, so we use the default fail code, and format err
			// to string and set to stderr
			log.Printf("Could not get exit code for failed program: %v, %v", name, args)
			exitCode = defaultFailedCode
			if stderr == "" {
				stderr = err.Error()
			}
		}
	} else {
		// success, exitCode should be 0 if go is ok
		ws := cmd.ProcessState.Sys().(syscall.WaitStatus)
		exitCode = ws.ExitStatus()
	}
	log.Printf("command result: stdout: %v, stderr: %v, exitCode: %v", stdout, stderr, exitCode)
	return stdout,stderr,exitCode
}
func retryLogic(attempts int, sleep time.Duration, string_to_pass string,f func(string) error) (err error) {
	for i := 0; ; i++ {
		err = f(string_to_pass)
		if err == nil {
			fmt.Println("Successfully found the file.. Starting the upload process")
			return
		}

		if i >= (attempts - 1) {
			break
		}
		sleep := time.Duration(sleep)*time.Second
		time.Sleep(sleep)

		log.Println("retrying after error:", err)
	}
	return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

func exitErrorf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}

func upload_to_s3(filename string,bucket string,key string){
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	sess,_= session.NewSession(&aws.Config{
		Region: aws.String("us-east-2")},
	)
	file, err := os.Open(filename)
	if err != nil {
		exitErrorf("Unable to open file %q, %v", err)
	}

	defer file.Close()

	uploader := s3manager.NewUploader(sess)
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key: aws.String(key+filename),
		Body: file,
	})
	if err != nil {
		exitErrorf("Unable to upload %q to %q, %v", filename, bucket, err)
	}

	fmt.Printf("Successfully uploaded %q to %q\n", filename, bucket)
}

func remove_bucket_name_from_key(key string,bucket string)string{
	return strings.Replace(key,"s3://"+bucket+"/","",-1)
}

func download_from_s3(bucket string ,key string,location string){
	splitItem := strings.Split(key,"/")
	item := splitItem[len(splitItem )-1]
	file, err := os.Create(location+"/"+item)
	if err != nil {
		exitErrorf("Unable to open file %q, %v", err)
	}

	defer file.Close()

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	sess, err = session.NewSession(&aws.Config{
		Region: aws.String("us-east-2")},
	)
	downloader := s3manager.NewDownloader(sess)

	numBytes, err := downloader.Download(file,
		&s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
	if err != nil {
		exitErrorf("Unable to download item %q, %v", item, err)
	}

	fmt.Println("Downloaded", file.Name(), numBytes, "bytes")
}
func (ProductBuildMsg ProductBuild)GitCommandGenerator()string{
	CommonCommands := fmt.Sprintf("cd C:/%s/ecdn/ && git fetch && git checkout -f %s && git reset --hard origin/%s && git clean -xfd && Build/scripts/",ProductBuildMsg.User,ProductBuildMsg.Commit,ProductBuildMsg.Commit)
    return CommonCommands
}
func (ProductBuildMsg ProductBuild)buildAltimeter(){
	ZipFilePath := fmt.Sprintf("%s",TempLocationToStoreBinaries)
	download_from_s3(ProductBuildMsg.Bucket,ProductBuildMsg.SourceWithOutBucketName,ZipFilePath)
    CommandToRun := ProductBuildMsg.GitCommandGenerator()+fmt.Sprintf("build-altimeter-win -z %s",ZipFilePath)
    log.Println(CommandToRun)
	//RunCommand("sh","-c",CommandToRun)
}
func (ProductBuildMsg ProductBuild)buildMulticastplusSender(){
	ZipFilePath := fmt.Sprintf("%s",TempLocationToStoreBinaries)
	download_from_s3(ProductBuildMsg.Bucket,ProductBuildMsg.SourceWithOutBucketName,ZipFilePath)
	CommandToRun := ProductBuildMsg.GitCommandGenerator()+fmt.Sprintf("build-multicastplus-win -z %s",ZipFilePath)
	log.Println(CommandToRun)
	//RunCommand("sh","-c",CommandToRun)
}
func (ProductBuildMsg ProductBuild)buildOmnicache(){
	ZipFilePath := fmt.Sprintf("%s",TempLocationToStoreBinaries)
	download_from_s3(ProductBuildMsg.Bucket,ProductBuildMsg.SourceWithOutBucketName,ZipFilePath)
	CommandToRun := ProductBuildMsg.GitCommandGenerator()+fmt.Sprintf("build-omnicache-win -z %s",ZipFilePath)
	log.Println(CommandToRun)
	//RunCommand("sh","-c",CommandToRun)
}
func (ProductBuildMsg ProductBuild)buildMulticastplusReceiver(){
	CommandToRun := ProductBuildMsg.GitCommandGenerator()+fmt.Sprintf("build-receiver-win")
	log.Println(CommandToRun)
	//RunCommand("sh","-c",CommandToRun)
}
func move_dir(sourceDir string,DestinationDir string ){
	cmd :=exec.Command("mv",sourceDir,DestinationDir)
	out,err := cmd.CombinedOutput()
	fmt.Println(string(out),err)
}
func wait_for_file_to_exist(path string) error{
	if _, err := os.Stat(path); err == nil {
		return nil

	} else if os.IsNotExist(err) {
		return errors.New("FileNotFoundError")

	} else {
		fmt.Println(err)
	}
	return nil
}
func rand_num_generator()int{
	seed := rand.NewSource(time.Now().UnixNano())
	rand1 := rand.New(seed)
	return rand1.Intn(30000)
}
func (ProductBuildMsg ProductBuild)CallingTheAppropriateBuild(){
	switch ProductBuildMsg.Product{
	case "multicastplus-receiver":
		log.Println("Building Multicastplus Receiver")
		go ProductBuildMsg.buildMulticastplusReceiver()
	case "multicastplus-sender":
		log.Println("Building Multicastplus Sender")
		go ProductBuildMsg.buildMulticastplusSender()
	case "omnicache":
		log.Println("Building Omincache")
		go ProductBuildMsg.buildOmnicache()
	case "altimeter":
		log.Println("Building Altimeter")
		go ProductBuildMsg.buildAltimeter()
	default:
		log.Printf("Wrong Product in Message: %s",ProductBuildMsg.Product)
	}
}
func (ProductBuildMsg *ProductBuild)InitializeParams(message string){
	if err := json.Unmarshal([]byte(message),&ProductBuildMsg); err != nil {
		panic(err)
	}
	ProductBuildMsg.Bucket = "ramp-build"
	SourceToDownloadBinaries := ProductBuildMsg.Binaries
	DestinationToSendBuiltProducts := ProductBuildMsg.Destination
	ProductBuildMsg.SourceWithOutBucketName = remove_bucket_name_from_key(SourceToDownloadBinaries,ProductBuildMsg.Bucket)
	ProductBuildMsg.DestinationWithOutBucketName = remove_bucket_name_from_key(DestinationToSendBuiltProducts,ProductBuildMsg.Bucket)
	ProductBuildMsg.CallingTheAppropriateBuild()
}

func main() {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	sess, _ = session.NewSession(&aws.Config{
		Region: aws.String("us-east-2")},
	)

	svc := sqs.New(sess)
	qURL := "https://sqs.us-east-2.amazonaws.com/726943805616/test-parallel-process"//***change the queue here****
	for {
		result, err := svc.ReceiveMessage(&sqs.ReceiveMessageInput{
			AttributeNames: []*string{
				aws.String(sqs.MessageSystemAttributeNameSentTimestamp),
			},
			MessageAttributeNames: []*string{
				aws.String(sqs.QueueAttributeNameAll),
			},
			QueueUrl:            &qURL,
			MaxNumberOfMessages: aws.Int64(1),
			VisibilityTimeout:   aws.Int64(20), // 20 seconds
			WaitTimeSeconds:     aws.Int64(0),
		})

		if err != nil {
			fmt.Println("Error", err)

		}


		fmt.Printf("Received %d messages.\n", len(result.Messages))
		if len(result.Messages) > 0 {
			fmt.Println(*result.Messages[0].Body)
			//json.Unmarshal([]byte(*result.Messages[0].Body),&CodeSignMsg)
			var ProductBuildMsg ProductBuild
			go ProductBuildMsg.InitializeParams(*result.Messages[0].Body)
			resultDelete, err := svc.DeleteMessage(&sqs.DeleteMessageInput{
				QueueUrl:      &qURL,
				ReceiptHandle: result.Messages[0].ReceiptHandle,
			})

			if err != nil {
				fmt.Println("Delete Error", err)
				return
			}
			fmt.Println("Message Deleted", resultDelete)
		}

		if len(result.Messages) == 0 {
			fmt.Println("Received no messages")
			time.Sleep(5*time.Second)

		}



	}}
