package athq

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type clients struct {
	athena *athena.Client
	s3     *s3.Client
	region string
}

func newClients(ctx context.Context) (*clients, error) {
	var optFns []func(*config.LoadOptions) error
	if opts.region != "" {
		optFns = append(optFns, config.WithRegion(opts.region))
	}
	if opts.profile != "" {
		optFns = append(optFns, config.WithSharedConfigProfile(opts.profile))
	}
	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return &clients{
		athena: athena.NewFromConfig(cfg),
		s3:     s3.NewFromConfig(cfg),
		region: cfg.Region,
	}, nil
}

// signalContext returns a context that is canceled on SIGINT/SIGTERM so a
// running query can be stopped on the Athena side as well.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
