package main

import (
	"context"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(handler)
}

func handler(ctx context.Context) error {

	time.Sleep(2 * time.Second)

	return nil
}
