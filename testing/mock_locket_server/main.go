package main

//
// This mock locket server was imported
// wholesale from paas-rds-metric-collector
//

import (
	"crypto/tls"
	"os"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/locket/grpcserver"
	"code.cloudfoundry.org/locket/models"
	"errors"
	"flag"
	"fmt"
	"github.com/tedsuo/ifrit"
	gcontext "golang.org/x/net/context"
	"path"
	"sync"
)

var (
	fixturesPath  string
	lockingMode   string
	listenAddress string
)

func init() {
	flag.StringVar(&fixturesPath, "fixturesPath", "", "Path to a directory containing client.{crt,key}")
	flag.StringVar(&lockingMode, "mode", "alwaysGrantLock", "Determines the locking behaviour")
	flag.StringVar(&listenAddress, "listenAddress", "", "The host and port to listen on. Example: 0.0.0.0:8891")
}

func main() {
	flag.Parse()

	logger := lager.NewLogger("grpc")
	logger.RegisterSink(lager.NewWriterSink(os.Stdout, lager.DEBUG))

	logger.Debug("LockingMode: " + lockingMode)
	var handler = testHandler{
		mode:          lockingMode,
		keyBasedLocks: make(map[string]string, 0),
	}
	certificate, err := tls.LoadX509KeyPair(
		path.Join(fixturesPath, "locket-server.cert.pem"),
		path.Join(fixturesPath, "locket-server.key.pem"),
	)
	if err != nil {
		logger.Error("Error loading certs", err)
		os.Exit(1)
	}
	grpcServer := grpcserver.NewGRPCServer(logger, listenAddress, &tls.Config{
		Certificates: []tls.Certificate{certificate},
	}, &handler)
	err = <-ifrit.Invoke(grpcServer).Wait()
	if err != nil {
		logger.Error("exited-with-failure", err)
		os.Exit(1)
	}
}

type testHandler struct {
	mode      string
	lockCount int

	kblMux        sync.Mutex
	keyBasedLocks map[string]string
}

func (h *testHandler) Lock(ctx gcontext.Context, req *models.LockRequest) (*models.LockResponse, error) {
	h.lockCount++
	switch h.mode {
	case "alwaysGrantLock":
		return &models.LockResponse{}, nil
	case "neverGrantLock":
		return nil, models.ErrLockCollision
	case "grantLockAfterFiveAttempts":
		if h.lockCount <= 5 {
			return nil, models.ErrLockCollision
		} else {
			return &models.LockResponse{}, nil
		}
	case "grantLockOnceThenFail":
		if h.lockCount <= 1 {
			return &models.LockResponse{}, nil
		} else {
			return nil, models.ErrLockCollision
		}
	case "keyBasedLock":
		h.kblMux.Lock()
		defer h.kblMux.Unlock()

		resource := req.Resource
		currentOwner, present := h.keyBasedLocks[resource.Key]

		if !present {
			h.keyBasedLocks[resource.Key] = currentOwner
			return &models.LockResponse{}, nil
		}

		if currentOwner == resource.Owner {
			return &models.LockResponse{}, nil
		}

		return nil, models.ErrLockCollision
	default:
		return nil, errors.New(fmt.Sprintf("Unexpected mode %s", h.mode))
	}
}

func (h *testHandler) Release(ctx gcontext.Context, req *models.ReleaseRequest) (*models.ReleaseResponse, error) {
	switch h.mode {
	case "keyBasedLock":
		h.kblMux.Lock()
		defer h.kblMux.Unlock()

		resource := req.Resource

		delete(h.keyBasedLocks, resource.Key)

		return &models.ReleaseResponse{}, nil
	default:
		return &models.ReleaseResponse{}, nil
	}
}
func (h *testHandler) Fetch(ctx gcontext.Context, req *models.FetchRequest) (*models.FetchResponse, error) {
	return &models.FetchResponse{}, nil
}
func (h *testHandler) FetchAll(ctx gcontext.Context, req *models.FetchAllRequest) (*models.FetchAllResponse, error) {
	return &models.FetchAllResponse{}, nil
}
