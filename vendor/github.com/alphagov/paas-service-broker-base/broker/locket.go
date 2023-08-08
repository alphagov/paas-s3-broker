package broker

import (
	"context"
	"sync"
	"time"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/locket"
	locket_models "code.cloudfoundry.org/locket/models"
	"google.golang.org/grpc"
)

var (
	// DefaultLocketRetryInterval is the time between retries while waiting
	// for successful locket response
	DefaultLocketRetryInterval = time.Second * 1
)

type LocketConfig struct {
	Address        string `json:"address"`
	CACertFile     string `json:"ca_cert_file"`
	ClientCertFile string `json:"client_cert_file"`
	ClientKeyFile  string `json:"client_key_file"`
	SkipVerify     bool
	RetryInterval  time.Duration
}

type ServiceLock struct {
	ServiceName string
	Key         string
	Owner       string
}

func newLocketClient(cfg *LocketConfig, logger lager.Logger) (locket_models.LocketClient, error) {
	locketSession := logger.Session("locket")
	locketConfig := locket.ClientLocketConfig{
		LocketAddress:        cfg.Address,
		LocketCACertFile:     cfg.CACertFile,
		LocketClientCertFile: cfg.ClientCertFile,
		LocketClientKeyFile:  cfg.ClientKeyFile,
	}

	if cfg.SkipVerify {
		return locket.NewClientSkipCertVerify(locketSession, locketConfig)
	} else {
		return locket.NewClient(locketSession, locketConfig)
	}
}

type SimpleLock struct {
	mu sync.Mutex
}

func (l *SimpleLock) Lock(ctx context.Context, in *locket_models.LockRequest, opts ...grpc.CallOption) (*locket_models.LockResponse, error) {
	l.mu.Lock()
	return nil, nil
}
func (l *SimpleLock) Fetch(ctx context.Context, in *locket_models.FetchRequest, opts ...grpc.CallOption) (*locket_models.FetchResponse, error) {
	return nil, nil
}
func (l *SimpleLock) Release(ctx context.Context, in *locket_models.ReleaseRequest, opts ...grpc.CallOption) (*locket_models.ReleaseResponse, error) {
	l.mu.Unlock()
	return nil, nil
}
func (l *SimpleLock) FetchAll(ctx context.Context, in *locket_models.FetchAllRequest, opts ...grpc.CallOption) (*locket_models.FetchAllResponse, error) {
	return nil, nil
}
