package mock_locket_server

//
// This mock locket server was imported
// wholesale from paas-rds-metric-collector
// which was modified in paas-s3-broker
// and once again here
//

import (
	"crypto/tls"
	"os"
	"path"
	"syscall"

	"errors"
	"fmt"
	"sync"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/locket/grpcserver"
	"code.cloudfoundry.org/locket/models"
	"github.com/phayes/freeport"
	"github.com/tedsuo/ifrit"
	gcontext "golang.org/x/net/context"
)

type MockLocket struct {
	Logger        lager.Logger
	ListenAddress string
	Certificate   tls.Certificate
	LockingMode   string
	Handler       testHandler
	Process       ifrit.Process
}

func New(lockingMode string, fixturesPath string) (*MockLocket, error) {
	logger := lager.NewLogger("grpc")
	logger.RegisterSink(lager.NewWriterSink(os.Stdout, lager.DEBUG))

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
		return nil, err
	}

	port, err := freeport.GetFreePort()
	if err != nil {
		logger.Error("Error getting TCP port", err)
		return nil, err
	}
	listenAddress := fmt.Sprintf("127.0.0.1:%d", port)

	logger.Debug("listen", lager.Data{
		"address":      listenAddress,
		"locking-mode": lockingMode,
	})

	return &MockLocket{
		Logger:        logger,
		ListenAddress: listenAddress,
		Certificate:   certificate,
		LockingMode:   lockingMode,
		Handler:       handler,
	}, nil
}

func (m *MockLocket) Start(logger lager.Logger, listenAddress string, certificate tls.Certificate, handler testHandler) {
	grpcServer := grpcserver.NewGRPCServer(logger, listenAddress, &tls.Config{
		Certificates: []tls.Certificate{certificate},
	}, &handler)
	m.Process = ifrit.Invoke(grpcServer)
	<-m.Process.Ready()
}

func (m *MockLocket) Stop() {
	m.Process.Signal(syscall.SIGKILL)
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
