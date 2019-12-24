package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/ec2"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func retry(attempts int, sleep time.Duration, string_to_pass string,f func(string) error) (err error) {
	for i := 0; ; i++ {
		err = f(string_to_pass)
		if err == nil {
			fmt.Println("Successfully deleted the subnets")
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

func check(e error) {
	if e != nil {
		panic(e)
	}
}
func DeleteDynamoDBID(table string ,KeyToDelete string,ValueToDelete int){
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-2")},
	)
	m:= make(map[string]int)
	m[KeyToDelete]=ValueToDelete
	av, err := dynamodbattribute.MarshalMap(m)
	dyndb := dynamodb.New(sess)
	input := &dynamodb.DeleteItemInput{
		Key: av,
		TableName: aws.String(table),
	}
	fmt.Println(input)
	_, err = dyndb.DeleteItem(input)
	if err != nil {
		fmt.Println("Got error calling DeleteItem")
		fmt.Println(err.Error())
		return
	}

}
func DeleteSubnetID(SubnetID string ) error {
	cmd := exec.Command("aws" , "--region","us-east-2","ec2","delete-subnet", "--subnet-id" ,SubnetID)
	out,_ := cmd.CombinedOutput()
	output_string := string(out)
	if strings.Contains(output_string,"A client error")|| strings.Contains(output_string,"subnets cannot be deleted") {
		return errors.New("SubnetNotDeletableError")
	}

	return nil
}
func DeleteRoute53Records(DeploymentID string) {

	cmd := exec.Command("aws" ,"--region", "us-east-2", "route53" ,"list-resource-record-sets", "--hosted-zone-id=/hostedzone/Z247B00TLFWFX1")
	route53outmap := make(map[string]interface{})
	out,_ := cmd.CombinedOutput()

	err := json.Unmarshal([]byte(string(out)),&route53outmap)
	if err != nil {
		panic(err)
	}

	type RecordSets struct {
		DnsRecords []map[string]string `json:"ResourceRecords"`
		TypeOfRecord string `json:"Type"`
		NameofRecord string `json:"Name"`
		TimeToLive int `json:"TTL"`
	}

	type Records struct {
		Resourcerecordsets []RecordSets `json:"ResourceRecordSets"`
	}

	type changes struct{
		Action string
		ResourceRecordSet RecordSets
	}
	type deletednsrecord struct{
		Comment string
		Changes []changes
	}


	var records Records
	if err := json.Unmarshal([]byte(string(out)),&records); err != nil {
		panic(err)
	}
	var DelDnsRecords deletednsrecord
	var DnsrecordsTodelete changes
	for _,element:= range records.Resourcerecordsets{
		if strings.Contains(element.NameofRecord,DeploymentID){
			split1 := strings.Split(element.NameofRecord,".")
			split2 := strings.Split(split1[0],"-")
			if(strings.Contains(DeploymentID,split2[len(split2)-1])){
				DnsrecordsTodelete.ResourceRecordSet = element
				DnsrecordsTodelete.Action = "DELETE"
				DelDnsRecords.Changes = append(DelDnsRecords.Changes,DnsrecordsTodelete)
			}


		}


	}
	DelDnsRecords.Comment = "testing deleting dns records"
	myjson, _ := json.Marshal(DelDnsRecords)
	fmt.Println(string(myjson))
	if runtime.GOOS == "windows" {
		json_file, err := os.Create("C:/Temp/tmp_json")//only works ion windows
		defer json_file.Close()
		_,err = json_file.Write(myjson)
		check(err)
		cmd = exec.Command("aws" ,"--region", "us-east-2", "route53" ,"change-resource-record-sets", "--hosted-zone-id=/hostedzone/Z247B00TLFWFX1","--change-batch","file://C:/Temp/tmp_json")
		out,_ = cmd.CombinedOutput()
		fmt.Println(string(out))

	} else {
		json_file, err := os.Create("/tmp/tmp_json")//only works ion windows
		defer json_file.Close()
		_,err = json_file.Write(myjson)
		check(err)
		cmd = exec.Command("aws" ,"--region", "us-east-2", "route53" ,"change-resource-record-sets", "--hosted-zone-id=/hostedzone/Z247B00TLFWFX1","--change-batch","file:///tmp/tmp_json")
		out,_ = cmd.CombinedOutput()
		fmt.Println(string(out))

	}



}
func TerminateInstances(Instances []*string){
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-2")},
	)
	ec2Svc := ec2.New(sess)
	input := &ec2.TerminateInstancesInput{
		InstanceIds: Instances,
	}
	result, err := ec2Svc.TerminateInstances(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			fmt.Println(err.Error())
		}
		return
	}

	fmt.Println(result)




}
func GetInfoAboutInstaces(deployment_id string)([]string,string,int){

	var InstancesToTerminate[] string
	var SubnetToTerminate string
	var PrivateIPAdd string
	var int_subnet_id int
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-2")},
	)
	ec2Svc := ec2.New(sess)
	result1, err := ec2Svc.DescribeInstances(nil)
	if err != nil {
		fmt.Println("Error", err)
	} else {
		for idx := range result1.Reservations {
			for _, inst := range result1.Reservations[idx].Instances {
				for _,tag_value := range(inst.Tags){



					if *tag_value.Key == "Name" {
						split_string := strings.Split(*tag_value.Value,"-")
						if(strings.Contains(deployment_id,split_string[len(split_string)-1])){
							if *inst.State.Name != "terminated"{
							fmt.Println(*inst)
							SubnetToTerminate = *inst.SubnetId
							PrivateIPAdd = *inst.PrivateIpAddress
							InstancesToTerminate = append(InstancesToTerminate,*inst.InstanceId)
						}}
					}
				}

			}}
		if PrivateIPAdd == ""{
			fmt.Println("Wrong Deployment ID given... No instances were found")
			os.Exit(1)
		}else{
			fmt.Println(PrivateIPAdd)
			subnet_id := strings.Split(PrivateIPAdd,".")[2]
			int_subnet_id,_ = strconv.Atoi(subnet_id)
		}



}
	return InstancesToTerminate,SubnetToTerminate,int_subnet_id
}

func GetInfoAboutUnprotectedInstances()([]string,string,int){

	var InstancesToTerminate[] string
	var SubnetToTerminate string
	var PrivateIPAdd string
	var int_subnet_id int
	//var deployment_id string
	var InstanceTags map[string]string
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-2")},
	)
	ec2Svc := ec2.New(sess)
	result1, err := ec2Svc.DescribeInstances(nil)
	if err != nil {
		fmt.Println("Error", err)
	} else {
		Branch := "Branch"
		Name := "Name"
		for idx := range result1.Reservations {
			for _, inst := range result1.Reservations[idx].Instances {
				InstanceTags = make(map[string]string)
				for _,tag_value := range(inst.Tags){

					InstanceTags[*tag_value.Key] = *tag_value.Value
				}
					for k,_ := range InstanceTags{
						if k == "Branch"{
							fmt.Println(InstanceTags[Branch])
							fmt.Println(InstanceTags[Name])
						}

					}



				}


			}

			}}
		if PrivateIPAdd == ""{
			fmt.Println("Wrong Deployment ID given... No instances were found")
			os.Exit(1)
		}else{
			fmt.Println(PrivateIPAdd)
			subnet_id := strings.Split(PrivateIPAdd,".")[2]
			int_subnet_id,_ = strconv.Atoi(subnet_id)
		}




	return InstancesToTerminate,SubnetToTerminate,int_subnet_id
}
func main() {

	if len(os.Args) > 1 {
		var deployment_id string = os.Args[1]
		fmt.Println("Got deployment ID:", deployment_id)
		Instaces_To_terminate, subnet_to_terminate, subnet_id := GetInfoAboutInstaces(deployment_id)
		TerminateInstances(aws.StringSlice(Instaces_To_terminate))
		fmt.Println("Successfully Terminated the instances")
		go DeleteRoute53Records(deployment_id)
		go DeleteDynamoDBID("atf_subnet_reservation", "subnet_index", subnet_id)
		time.Sleep(25 * time.Second)
		fmt.Println("Waiting for 25 seconds")
		err := retry(5, 5, subnet_to_terminate, DeleteSubnetID)
		fmt.Println("Got Error", err)
	} else{
		GetInfoAboutUnprotectedInstances()

	}
	}
