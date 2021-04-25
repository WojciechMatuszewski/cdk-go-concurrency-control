package main

import (
	"fmt"
	"os"
	"path"

	"github.com/aws/aws-cdk-go/awscdk"
	"github.com/aws/aws-cdk-go/awscdk/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/awss3assets"
	"github.com/aws/aws-cdk-go/awscdk/awsstepfunctions"
	"github.com/aws/aws-cdk-go/awscdk/awsstepfunctionstasks"
	"github.com/aws/constructs-go/constructs/v3"
	"github.com/aws/jsii-runtime-go"
)

type ConcurrencyControlStackProps struct {
	awscdk.StackProps
}

func NewConcurrencyControlStack(scope constructs.Construct, id string, props *ConcurrencyControlStackProps) awscdk.Stack {
	var sprops awscdk.StackProps
	if props != nil {
		sprops = props.StackProps
	}
	stack := awscdk.NewStack(scope, &id, &sprops)

	doWorkLambda := awslambda.NewFunction(stack, jsii.String("doWork"), &awslambda.FunctionProps{
		Timeout: awscdk.Duration_Seconds(jsii.Number(20)),
		Tracing: awslambda.Tracing_ACTIVE,
		Code: awslambda.AssetCode_FromAsset(jsii.String(path.Join(functionsDir(), "perform-work")), &awss3assets.AssetOptions{
			Bundling: &awscdk.BundlingOptions{
				Image: awslambda.Runtime_GO_1_X().BundlingDockerImage(),
				Command: &[]*string{
					jsii.String("bash"),
					jsii.String("-c"),
					jsii.String("go build -o /asset-output/main"),
				},
				User: jsii.String("root"),
			},
		}),
		Runtime: awslambda.Runtime_GO_1_X(),
		Handler: jsii.String("main"),
	})

	lockTable := awsdynamodb.NewTable(stack, jsii.String("lockTable"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("lockName"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		BillingMode:   awsdynamodb.BillingMode_PAY_PER_REQUEST,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})

	testSemaphoreMachineLogGroup := awslogs.NewLogGroup(stack, jsii.String("testSemaphoreLogGroup"), &awslogs.LogGroupProps{
		LogGroupName:  jsii.String("testSemaphoreLogGroup"),
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
		Retention:     awslogs.RetentionDays_ONE_DAY,
	})

	createLockStep := awsstepfunctionstasks.NewDynamoPutItem(stack, jsii.String("createLock"), &awsstepfunctionstasks.DynamoPutItemProps{
		IntegrationPattern: awsstepfunctions.IntegrationPattern_RUN_JOB,
		Item: &map[string]awsstepfunctionstasks.DynamoAttributeValue{
			"lockName":  awsstepfunctionstasks.DynamoAttributeValue_FromString(jsii.String("concurrentLock")),
			"lockCount": awsstepfunctionstasks.DynamoAttributeValue_FromNumber(jsii.Number(0)),
		},
		Table:               lockTable,
		ConditionExpression: jsii.String("attribute_not_exists(#lockName)"),
		ExpressionAttributeNames: &map[string]*string{
			"#lockName": jsii.String("lockName"),
		},
	})

	waitForLock := awsstepfunctions.NewWait(stack, jsii.String("waitForLock"), &awsstepfunctions.WaitProps{
		Time: awsstepfunctions.WaitTime_Duration(awscdk.Duration_Seconds(jsii.Number(3))),
	})

	acquireLockStep := awsstepfunctionstasks.NewDynamoUpdateItem(stack, jsii.String("acquireLock"), &awsstepfunctionstasks.DynamoUpdateItemProps{
		IntegrationPattern: awsstepfunctions.IntegrationPattern_RUN_JOB,
		Key: &map[string]awsstepfunctionstasks.DynamoAttributeValue{
			"lockName": awsstepfunctionstasks.DynamoAttributeValue_FromString(jsii.String("concurrentLock")),
		},
		Table:               lockTable,
		ConditionExpression: jsii.String("#lockCount <> :lockLimit"),
		ExpressionAttributeNames: &map[string]*string{
			"#lockCount":  jsii.String("lockCount"),
			"#lastHolder": jsii.String("lastHolder"),
		},
		ExpressionAttributeValues: &map[string]awsstepfunctionstasks.DynamoAttributeValue{
			":inc":        awsstepfunctionstasks.DynamoAttributeValue_FromNumber(jsii.Number(1)),
			":lockLimit":  awsstepfunctionstasks.DynamoAttributeValue_FromNumber(jsii.Number(5)),
			":lastHolder": awsstepfunctionstasks.DynamoAttributeValue_FromString(awsstepfunctions.JsonPath_StringAt(jsii.String("$$.Execution.Id"))),
		},
		ReturnValues:     awsstepfunctionstasks.DynamoReturnValues_ALL_NEW,
		UpdateExpression: jsii.String("SET #lockCount = #lockCount + :inc, #lastHolder = :lastHolder"),
	}).AddRetry(&awsstepfunctions.RetryProps{
		Errors: &[]*string{
			jsii.String("DynamoDB.AmazonDynamoDBException"),
		},
		MaxAttempts: jsii.Number(0),
	}).AddCatch(createLockStep, &awsstepfunctions.CatchProps{
		Errors: &[]*string{
			jsii.String("DynamoDB.AmazonDynamoDBException"),
		},
	}).AddCatch(waitForLock, &awsstepfunctions.CatchProps{
		Errors: &[]*string{
			jsii.String("DynamoDB.ConditionalCheckFailedException"),
		},
	})

	waitForLock.Next(acquireLockStep)

	releaseLock := awsstepfunctionstasks.NewDynamoUpdateItem(stack, jsii.String("releaseLock"), &awsstepfunctionstasks.DynamoUpdateItemProps{
		IntegrationPattern: awsstepfunctions.IntegrationPattern_RUN_JOB,
		Key: &map[string]awsstepfunctionstasks.DynamoAttributeValue{
			"lockName": awsstepfunctionstasks.DynamoAttributeValue_FromString(jsii.String("concurrentLock")),
		},
		Table:               lockTable,
		ConditionExpression: jsii.String("attribute_exists(#lockName) AND #lockCount > :zero"),
		ExpressionAttributeNames: &map[string]*string{
			"#lockCount": jsii.String("lockCount"),
			"#lockName":  jsii.String("lockName"),
		},
		ExpressionAttributeValues: &map[string]awsstepfunctionstasks.DynamoAttributeValue{
			":zero": awsstepfunctionstasks.DynamoAttributeValue_FromNumber(jsii.Number(0)),
			":dec":  awsstepfunctionstasks.DynamoAttributeValue_FromNumber(jsii.Number(-1)),
		},
		ReturnValues:     awsstepfunctionstasks.DynamoReturnValues_ALL_NEW,
		UpdateExpression: jsii.String("SET #lockCount = #lockCount + :dec"),
	})

	createLockStep.AddCatch(acquireLockStep, &awsstepfunctions.CatchProps{
		Errors: &[]*string{
			jsii.String("States.ALL"),
		},
		ResultPath: nil,
	}).Next(acquireLockStep)

	semaphoreMachine := awsstepfunctions.NewStateMachine(stack, jsii.String("semaphore"), &awsstepfunctions.StateMachineProps{
		Definition: acquireLockStep.Next(
			awsstepfunctionstasks.NewLambdaInvoke(stack, jsii.String("performWork"), &awsstepfunctionstasks.LambdaInvokeProps{
				IntegrationPattern: awsstepfunctions.IntegrationPattern_REQUEST_RESPONSE,
				LambdaFunction:     doWorkLambda,
			})).Next(releaseLock),
		StateMachineType: awsstepfunctions.StateMachineType_STANDARD,
		TracingEnabled:   jsii.Bool(true),
	})

	iterations := func() []string {
		iters := make([]string, 100)
		for i := 0; i < len(iters); i++ {
			iters[i] = fmt.Sprint(i)
		}

		return iters
	}()
	createPayloadDefinition := awsstepfunctions.NewPass(stack, jsii.String("preparePayload"), &awsstepfunctions.PassProps{
		Parameters: &map[string]interface{}{
			"iterations": iterations,
		},
	})

	runSemaphoreDefinition := awsstepfunctionstasks.NewStepFunctionsStartExecution(stack, jsii.String("invokeSemaphore"), &awsstepfunctionstasks.StepFunctionsStartExecutionProps{
		IntegrationPattern: awsstepfunctions.IntegrationPattern_RUN_JOB,
		StateMachine:       semaphoreMachine,
		Input: awsstepfunctions.TaskInput_FromObject(&map[string]interface{}{
			// Adds "startedBy" in the AWS console
			"AWS_STEP_FUNCTIONS_STARTED_BY_EXECUTION_ID": awsstepfunctions.JsonPath_StringAt(jsii.String("$$.Execution.Id")),
		}),
	})

	runSemaphoreParallel := awsstepfunctions.NewMap(stack, jsii.String("semaphoreParallel"), &awsstepfunctions.MapProps{
		ItemsPath: jsii.String("$.iterations"),
	}).Iterator(runSemaphoreDefinition)

	awsstepfunctions.NewStateMachine(stack, jsii.String("testSemaphore"), &awsstepfunctions.StateMachineProps{
		Definition: createPayloadDefinition.Next(runSemaphoreParallel),
		Logs: &awsstepfunctions.LogOptions{
			Destination:          testSemaphoreMachineLogGroup,
			IncludeExecutionData: jsii.Bool(true),
			Level:                awsstepfunctions.LogLevel_ALL,
		},
		StateMachineType: awsstepfunctions.StateMachineType_STANDARD,
		TracingEnabled:   jsii.Bool(true),
	})

	return stack
}

func main() {
	app := awscdk.NewApp(nil)

	NewConcurrencyControlStack(app, "ConcurrencyControlStack", &ConcurrencyControlStackProps{
		awscdk.StackProps{
			Env: env(),
		},
	})

	app.Synth(nil)
}

// env determines the AWS environment (account+region) in which our stack is to
// be deployed. For more information see: https://docs.aws.amazon.com/cdk/latest/guide/environments.html
func env() *awscdk.Environment {
	return &awscdk.Environment{
		Account: jsii.String(os.Getenv("CDK_DEFAULT_ACCOUNT")),
		Region:  jsii.String(os.Getenv("CDK_DEFAULT_REGION")),
	}
}

func functionsDir() string {
	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	return path.Join(pwd, "..", "functions")
}
