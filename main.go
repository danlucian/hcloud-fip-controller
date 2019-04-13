package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cbeneke/hcloud-fip-controller/pkg/fipcontroller"
)

func main() {
	client, err := fipcontroller.NewClient()
	if err != nil {
		fmt.Println(fmt.Errorf("could not initialise client: %v", err))
		os.Exit(1)
	}

	err = fipcontroller.Run(ctx, client)
	// TODO: Use channel with interrupt signal that blocks until it receives to cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err != nil {
		fmt.Println(fmt.Errorf("could not run client: %v", err))
		os.Exit(1)
	}
}
