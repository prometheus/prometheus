// build example

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
)

var clientCfg ClientConfig
var topicARN string

func init() {
	clientCfg.SetupFlags("", flag.CommandLine)

	flag.CommandLine.StringVar(&topicARN, "topic", "",
		"The topic `ARN` to send messages to")
}

func main() {
	if err := flag.CommandLine.Parse(os.Args[1:]); err != nil {
		flag.CommandLine.PrintDefaults()
		exitErrorf(err, "failed to parse CLI commands")
	}
	if len(topicARN) == 0 {
		flag.CommandLine.PrintDefaults()
		exitErrorf(errors.New("topic ARN required"), "")
	}

	httpClient := NewClient(clientCfg)
	sess, err := session.NewSession(&aws.Config{
		HTTPClient: httpClient,

		// Disable Retries to prevent the httptrace's getting mixed up on
		// retries.
		MaxRetries: aws.Int(0),
	})
	if err != nil {
		exitErrorf(err, "failed to load config")
	}

	// Start making the requests.
	svc := sns.New(sess)
	ctx := context.Background()

	fmt.Printf("Message: ")

	// Scan messages from the input with newline separators.
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		trace, err := publishMessage(ctx, svc, topicARN, scanner.Text())
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to publish message, %v", err)
		}
		RecordTrace(os.Stdout, trace)

		fmt.Println()
		fmt.Printf("Message: ")
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to read input, %v", err)
	}
}

// publishMessage will send the message to the SNS topic returning an request
// trace for metrics.
func publishMessage(ctx context.Context, svc *sns.SNS, topic, msg string) (*RequestTrace, error) {
	traceCtx := NewRequestTrace(ctx)
	defer traceCtx.RequestDone()

	_, err := svc.PublishWithContext(traceCtx, &sns.PublishInput{
		TopicArn: &topic,
		Message:  &msg,
	})
	if err != nil {
		return nil, err
	}

	return traceCtx, nil
}

func exitErrorf(err error, msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "FAILED: %v\n"+msg+"\n", append([]interface{}{err}, args...)...)
	os.Exit(1)
}
