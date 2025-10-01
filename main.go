package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

var Environment string

// Output result struct
type TaskResult struct {
	TaskArn       string `json:"task_arn"`
	StoppedReason string `json:"stopped_reason"`
	IsMaintenance bool   `json:"is_maintenance"`
	IsScaling     bool   `json:"is_scaling"`
}

// Splunk payload struct
type SplunkBody struct {
	Severity          string `json:"severity"`
	OriginatingMetric string `json:"originatingMetric"`
	Description       string `json:"description"`
	Status            string `json:"status"`
	Timestamp         string `json:"timestamp"`
	Detector          string `json:"detector"`
	Rule              string `json:"rule"`
	Inputs            struct {
		Signal struct {
			Value    string                 `json:"value"`
			Fragment string                 `json:"fragment"`
			Key      map[string]interface{} `json:"key"`
		} `json:"signal"`
	} `json:"inputs"`
	Dimensions struct {
		ServiceName string `json:"ServiceName"`
	} `json:"dimensions"`
}

// Helper function to check if stop reason is due to maintenance
func isMaintenance(reason string) bool {
	reason = strings.ToLower(reason)
	return strings.Contains(reason, reasonMaintenance)
}

// Helper function to check if stop reason is due to scaling
func isScaling(reason string) bool {
	reason = strings.ToLower(reason)
	return strings.Contains(reason, reasonScaling)
}

// sendPagerDutyAlert sends an alert to PagerDuty
func sendPagerDutyAlert(ctx context.Context, Severity, Detector string, splunk SplunkBody, taskArn string, secretValue string) error {
	// custom details map with information extracted from the Splunk event
	customDetails := map[string]interface{}{
		"ServiceName": splunk.Dimensions.ServiceName,
		"detector":    splunk.Detector,
		"inputs": map[string]interface{}{
			"signal": map[string]interface{}{
				"fragment": splunk.Inputs.Signal.Fragment,
				"key":      splunk.Inputs.Signal.Key,
				"value":    splunk.Inputs.Signal.Value,
			},
		},
		"rule":      splunk.Rule,
		"severity":  splunk.Severity,
		"status":    splunk.Status,
		"timestamp": splunk.Timestamp,
	}

	// Marshal to JSON for logging
	customDetailsJSON, err := json.MarshalIndent(customDetails, "", "  ")
	if err != nil {
		fmt.Printf("Failed to marshal customDetails: %v\n", err)
	} else {
		fmt.Printf("Custom Details:\n%s\n", string(customDetailsJSON))
	}

	// Construct the event payload
	event := map[string]interface{}{
		"routing_key":  secretValue,
		"event_action": "trigger",
		"payload": map[string]interface{}{
			"summary":        fmt.Sprintf("Critical Alert: %s (%s)", Severity, Detector),
			"source":         taskArn,
			"severity":       Severity,
			"detector":       Detector,
			"component":      "Eventing",
			"group":          "prod",
			"class":          "Availability",
			"custom_details": customDetails,
		},
	}

	// Marshal the payload into JSON
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal PagerDuty event: %w", err)
	}

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", "https://events.pagerduty.com/v2/enqueue", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create PagerDuty request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the request using an HTTP client
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send PagerDuty request: %w", err)
	}
	defer resp.Body.Close()

	// Read and print the response body
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("PagerDuty response status: %s, body: %s\n", resp.Status, string(body))

	// Check if the response status code is not 202 (Accepted)
	if resp.StatusCode != 202 {
		return fmt.Errorf("unexpected PagerDuty response status: %s", resp.Status)
	}

	return nil
}

type SecretsManagerAPI interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

// Fetch a secret from AWS Secrets Manager
func getSecret(ctx context.Context, client SecretsManagerAPI, secretName string) (string, error) {
	// Call GetSecretValue API
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName),
	}

	result, err := client.GetSecretValue(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve secret: %w", err)
	}

	// Ensure SecretString is not nil
	if result.SecretString == nil {
		return "", fmt.Errorf("secret value is empty or binary")
	}

	// Parse the secret string (assumes JSON format)
	var secretMap map[string]string
	err = json.Unmarshal([]byte(*result.SecretString), &secretMap)
	if err != nil {
		return "", fmt.Errorf("failed to parse secret string: %w", err)
	}

	// Extract the pagerduty_key value
	key := "pagerduty_key"
	value, exists := secretMap[key]
	if !exists {
		return "", fmt.Errorf("key %q not found in secret", key)
	}

	return value, nil
}

// Lambda handler
func handler(ctx context.Context, rawEvent json.RawMessage, client *ecs.Client, secretValue string) ([]TaskResult, error) {
	// Extract the body from the outer Lambda event
	var outerEvent struct {
		Body string `json:"body"`
	}

	if err := json.Unmarshal(rawEvent, &outerEvent); err != nil {
		return nil, fmt.Errorf("failed to parse outer event: %w", err)
	}

	// Parse the inner JSON body
	var splunk SplunkBody
	if err := json.Unmarshal([]byte(outerEvent.Body), &splunk); err != nil {
		return nil, fmt.Errorf("failed to parse inner body: %w", err)
	}

	fmt.Printf("----- SplunkBody Details -----\n")
	fmt.Printf("Severity : %s\n", splunk.Severity)
	fmt.Printf("OriginatingMetric : %s\n", splunk.OriginatingMetric)
	fmt.Printf("Description : %s\n", splunk.Description)
	fmt.Printf("Status : %s\n", splunk.Status)
	fmt.Printf("Timestamp : %s\n", splunk.Timestamp)
	fmt.Printf("Detector : %s\n", splunk.Detector)
	fmt.Printf("ServiceName : %s\n", splunk.Dimensions.ServiceName)

	// Validate service name
	if splunk.Dimensions.ServiceName == "" {
		return nil, fmt.Errorf("missing ServiceName in Splunk event")
	}

	// Step 0: Enforce Lambda handler timeout of 15 seconds
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Check for cancellation before ListTasks API call
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("operation canceled before ListTasks: %w", ctx.Err())
	default:
	}

	// Prepare customDetails map
	customDetails := map[string]interface{}{
		"ServiceName": splunk.Dimensions.ServiceName,
		"detector":    splunk.Detector,
		"inputs": map[string]interface{}{
			"signal": map[string]interface{}{
				"fragment": splunk.Inputs.Signal.Fragment,
				"key":      splunk.Inputs.Signal.Key,
				"value":    splunk.Inputs.Signal.Value,
			},
		},
		"rule":      splunk.Rule,
		"severity":  splunk.Severity,
		"status":    splunk.Status,
		"timestamp": splunk.Timestamp,
	}

	// Log custom details to CloudWatch
	customDetailsJSON, err := json.MarshalIndent(customDetails, "", "  ")
	if err != nil {
		fmt.Printf("Failed to marshal customDetails: %v\n", err)
	} else {
		fmt.Printf("Custom Details:\n%s\n", string(customDetailsJSON))
	}

	// Step 3: List recently stopped tasks for the input service in the specified ECS cluster
	listOut, err := client.ListTasks(ctx, &ecs.ListTasksInput{
		Cluster:      aws.String(Environment),
		ServiceName:  aws.String(splunk.Dimensions.ServiceName),
		DesiredStatus: ecsTypes.DesiredStatusStopped,
		MaxResults:   aws.Int32(10),
	})
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	// Step 4: Return error if no stopped tasks found
	if len(listOut.TaskArns) == 0 {
		fmt.Printf("No stopped tasks found for service %s in cluster %s\n", splunk.Dimensions.ServiceName, Environment)
		return []TaskResult{}, nil
	}

	// Check for cancellation before DescribeTasks API call
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("operation canceled before DescribeTasks: %w", ctx.Err())
	default:
	}

	// Step 5: Describe the stopped tasks to get detailed information
	descOut, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(Environment),
		Tasks:   listOut.TaskArns,
	})
	if err != nil {
		return nil, fmt.Errorf("describe tasks: %w", err)
	}

	var results []TaskResult
	for _, t := range descOut.Tasks {
		// Check for context cancelation inside the loop
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("operation canceled during loop: %w", ctx.Err())
		default:
		}

		taskArn := aws.ToString(t.TaskArn)
		reason := strings.ToLower(aws.ToString(t.StoppedReason))
		isMaint := isMaintenance(reason)
		isScale := isScaling(reason)

		results = append(results, TaskResult{
			TaskArn:       taskArn,
			StoppedReason: reason,
			IsMaintenance: isMaint,
			IsScaling:     isScale,
		})

		if !isMaint && !isScale {
			fmt.Printf(" pager duty sent: ")
			// Uncomment when ready to send
			// err := sendPagerDutyAlert(ctx, splunk.Severity, splunk.Detector, splunk, taskArn, secretValue)
			// if err != nil {
			// 	fmt.Printf(" Failed to send PagerDuty alert: %v\n", err)
			// } else {
			// 	fmt.Printf("PagerDuty alert sent for task %s\n", taskArn)
			// }
		}
	}

	return results, nil
}

// Main function to start the Lambda
func main() {
	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		fmt.Printf("Error loading AWS config: %v\n", err)
		return
	}

	secretClient := secretsmanager.NewFromConfig(cfg)
	ecsClient := ecs.NewFromConfig(cfg)
	Environment = os.Getenv("ENVIRONMENT")

	fmt.Print("environment value: ", Environment)

	secretValue, err := getSecret(ctx, secretClient, "visibility-eventing@"+Environment+"_secrets")
	if err != nil {
		fmt.Printf("Error fetching secret: %v\n", err)
		return
	}

	fmt.Printf("Fetched secret: %s\n", secretValue)

	// Wrap the handler to inject ecsClient
	lambda.Start(func(ctx context.Context, rawEvent json.RawMessage) ([]TaskResult, error) {
		return handler(ctx, rawEvent, ecsClient, secretValue)
	})
}


