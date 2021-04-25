package main

import (
	"context"
	"math/rand"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(handler)
}

func handler(ctx context.Context) error {

	seconds := rand.Intn(10-2) + 2
	time.Sleep(time.Duration(seconds) * time.Second)

	return nil
}
