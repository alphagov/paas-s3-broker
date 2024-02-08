package broker

import (
	"context"
	"fmt"
	"time"

	uuid "github.com/satori/go.uuid"

	"errors"

	"code.cloudfoundry.org/lager"
	locket_models "code.cloudfoundry.org/locket/models"
	"github.com/alphagov/paas-service-broker-base/provider"
	"github.com/pivotal-cf/brokerapi/domain"
	"github.com/pivotal-cf/brokerapi/domain/apiresponses"
)

var (
	ErrNilResponse                  = fmt.Errorf("invalid nil response from provider")
	ErrNotImplemented               = fmt.Errorf("provider does not implement this operation")
	ErrAsyncProvisionNotImplemented = fmt.Errorf("provider returned async response but does not implement AsyncProvisioner")
	ErrAsyncBindNotImplemented      = fmt.Errorf("provider returned async response but does not implement AsyncBinder")
)

var (
	// DefaultContextTimeout is the default timeout in which broker requests
	// (and therefore Provider implementations) should return
	DefaultContextTimeout = time.Second * 60
)

const (
	locketMaxTTL = 30
)

type Broker struct {
	config       Config
	Provider     provider.Provider
	logger       lager.Logger
	LocketClient locket_models.LocketClient
}

var _ domain.ServiceBroker = &Broker{}

func New(config Config, serviceProvider provider.Provider, logger lager.Logger) (*Broker, error) {
	b := &Broker{
		config:       config,
		Provider:     serviceProvider,
		logger:       logger,
		LocketClient: &SimpleLock{},
	}
	if config.API.Locket != nil {
		locketClient, err := newLocketClient(config.API.Locket, logger)
		if err != nil {
			return nil, err
		}
		b.LocketClient = locketClient
	}
	return b, nil
}

func (b *Broker) BinderImplemented() (provider.Binder, bool) {
	binder, ok := b.Provider.(provider.Binder)
	return binder, ok
}

func (b *Broker) AsyncBinderImplemented() (provider.AsyncBinder, bool) {
	asyncbinder, ok := b.Provider.(provider.AsyncBinder)
	return asyncbinder, ok
}

func (b *Broker) ProvisionerImplemented() (provider.Provisioner, bool) {
	provisioner, ok := b.Provider.(provider.Provisioner)
	return provisioner, ok
}

func (b *Broker) AsyncProvisionerImplemented() (provider.AsyncProvisioner, bool) {
	asyncprovisioner, ok := b.Provider.(provider.AsyncProvisioner)
	return asyncprovisioner, ok
}

func (b *Broker) UpdaterImplemented() (provider.Updater, bool) {
	updater, ok := b.Provider.(provider.Updater)
	return updater, ok
}

func (b *Broker) Services(ctx context.Context) ([]domain.Service, error) {
	services := []domain.Service{}
	for _, svc := range b.config.Catalog.Catalog.Services {
		if _, ok := b.BinderImplemented(); ok {
			svc.Bindable = true
		}
		if _, ok := b.AsyncBinderImplemented(); ok {
			svc.BindingsRetrievable = true
		}
		services = append(services, svc)
	}
	return services, nil
}

func (b *Broker) Provision(
	ctx context.Context,
	instanceID string,
	details domain.ProvisionDetails,
	asyncAllowed bool,
) (domain.ProvisionedServiceSpec, error) {
	b.logger.Debug("provision-start", lager.Data{
		"instance-id":   instanceID,
		"details":       details,
		"async-allowed": asyncAllowed,
	})

	if !asyncAllowed {
		return domain.ProvisionedServiceSpec{}, apiresponses.ErrAsyncRequired
	}

	service, err := findServiceByID(b.config.Catalog, details.ServiceID)
	if err != nil {
		return domain.ProvisionedServiceSpec{}, err
	}

	plan, err := findPlanByID(service, details.PlanID)
	if err != nil {
		return domain.ProvisionedServiceSpec{}, err
	}

	providerCtx, cancelFunc := context.WithTimeout(ctx, b.config.API.ContextTimeout())
	defer cancelFunc()

	lock, err := b.ObtainServiceLock(providerCtx, instanceID, locketMaxTTL)
	if err != nil {
		return domain.ProvisionedServiceSpec{}, err
	}
	defer b.ReleaseServiceLock(providerCtx, lock)

	provisionData := provider.ProvisionData{
		InstanceID: instanceID,
		Details:    details,
		Service:    service,
		Plan:       plan,
	}

	provisioner, ok := b.ProvisionerImplemented()
	if !ok {
		return domain.ProvisionedServiceSpec{}, ErrNotImplemented
	}

	res, err := provisioner.Provision(providerCtx, provisionData)
	if err != nil {
		return domain.ProvisionedServiceSpec{}, err
	}
	if res == nil {
		return domain.ProvisionedServiceSpec{}, ErrNilResponse
	}
	if _, asyncImplemented := b.AsyncProvisionerImplemented(); res.IsAsync && !asyncImplemented {
		return domain.ProvisionedServiceSpec{}, ErrAsyncProvisionNotImplemented
	}

	b.logger.Debug("provision-success", lager.Data{
		"instance-id":    instanceID,
		"details":        details,
		"operation-data": res.OperationData,
		"is-async":       res.IsAsync,
	})

	return *res, nil
}

func (b *Broker) Deprovision(
	ctx context.Context,
	instanceID string,
	details domain.DeprovisionDetails,
	asyncAllowed bool,
) (domain.DeprovisionServiceSpec, error) {
	b.logger.Debug("deprovision-start", lager.Data{
		"instance-id":   instanceID,
		"details":       details,
		"async-allowed": asyncAllowed,
	})

	if !asyncAllowed {
		return domain.DeprovisionServiceSpec{}, apiresponses.ErrAsyncRequired
	}

	providerCtx, cancelFunc := context.WithTimeout(ctx, b.config.API.ContextTimeout())
	defer cancelFunc()

	service, err := findServiceByID(b.config.Catalog, details.ServiceID)
	if err != nil {
		return domain.DeprovisionServiceSpec{}, err
	}

	plan, err := findPlanByID(service, details.PlanID)
	if err != nil {
		return domain.DeprovisionServiceSpec{}, err
	}

	lock, err := b.ObtainServiceLock(providerCtx, instanceID, locketMaxTTL)
	if err != nil {
		return domain.DeprovisionServiceSpec{}, err
	}
	defer b.ReleaseServiceLock(providerCtx, lock)

	deprovisionData := provider.DeprovisionData{
		InstanceID: instanceID,
		Service:    service,
		Plan:       plan,
		Details:    details,
	}

	provisioner, ok := b.ProvisionerImplemented()
	if !ok {
		return domain.DeprovisionServiceSpec{}, ErrNotImplemented
	}
	res, err := provisioner.Deprovision(providerCtx, deprovisionData)
	if err != nil {
		return domain.DeprovisionServiceSpec{}, err
	}
	if res == nil {
		return domain.DeprovisionServiceSpec{}, ErrNilResponse
	}
	if _, asyncImplemented := b.AsyncProvisionerImplemented(); res.IsAsync && !asyncImplemented {
		return domain.DeprovisionServiceSpec{}, ErrAsyncProvisionNotImplemented
	}

	b.logger.Debug("deprovision-success", lager.Data{
		"instance-id":    instanceID,
		"details":        details,
		"operation-data": res.OperationData,
		"is-async":       res.IsAsync,
	})

	return *res, nil
}

func (b *Broker) Bind(
	ctx context.Context,
	instanceID,
	bindingID string,
	details domain.BindDetails,
	asyncAllowed bool,
) (domain.Binding, error) {
	b.logger.Debug("binding-start", lager.Data{
		"instance-id": instanceID,
		"binding-id":  bindingID,
		"details":     details,
	})

	providerCtx, cancelFunc := context.WithTimeout(ctx, b.config.API.ContextTimeout())
	defer cancelFunc()

	lock, err := b.ObtainServiceLock(providerCtx, instanceID, locketMaxTTL)
	if err != nil {
		return domain.Binding{}, err
	}
	defer b.ReleaseServiceLock(providerCtx, lock)

	bindData := provider.BindData{
		InstanceID:   instanceID,
		BindingID:    bindingID,
		Details:      details,
		AsyncAllowed: asyncAllowed,
	}

	binder, ok := b.BinderImplemented()
	if !ok {
		return domain.Binding{}, ErrNotImplemented
	}
	res, err := binder.Bind(providerCtx, bindData)
	if err != nil {
		return domain.Binding{}, err
	}
	if res == nil {
		return domain.Binding{}, ErrNilResponse
	}
	if _, asyncImplemented := b.AsyncBinderImplemented(); res.IsAsync && !asyncImplemented {
		return domain.Binding{}, ErrAsyncBindNotImplemented
	}

	b.logger.Debug("binding-success", lager.Data{
		"instance-id": instanceID,
		"binding-id":  bindingID,
		"details":     details,
	})

	return *res, nil
}

func (b *Broker) Unbind(
	ctx context.Context,
	instanceID,
	bindingID string,
	details domain.UnbindDetails,
	asyncAllowed bool,
) (domain.UnbindSpec, error) {
	b.logger.Debug("unbinding-start", lager.Data{
		"instance-id": instanceID,
		"binding-id":  bindingID,
		"details":     details,
	})

	providerCtx, cancelFunc := context.WithTimeout(ctx, b.config.API.ContextTimeout())
	defer cancelFunc()

	lock, err := b.ObtainServiceLock(providerCtx, instanceID, locketMaxTTL)
	if err != nil {
		return domain.UnbindSpec{}, err
	}
	defer b.ReleaseServiceLock(providerCtx, lock)

	unbindData := provider.UnbindData{
		InstanceID:   instanceID,
		BindingID:    bindingID,
		Details:      details,
		AsyncAllowed: asyncAllowed,
	}

	binder, ok := b.BinderImplemented()
	if !ok {
		return domain.UnbindSpec{}, ErrNotImplemented
	}
	res, err := binder.Unbind(providerCtx, unbindData)
	if err != nil {
		return domain.UnbindSpec{}, err
	}
	if res == nil {
		return domain.UnbindSpec{}, ErrNilResponse
	}
	if _, asyncImplemented := b.AsyncBinderImplemented(); res.IsAsync && !asyncImplemented {
		return domain.UnbindSpec{}, ErrAsyncBindNotImplemented
	}

	b.logger.Debug("unbinding-success", lager.Data{
		"instance-id": instanceID,
		"binding-id":  bindingID,
		"details":     details,
	})

	return *res, nil
}

func (b *Broker) GetBinding(
	ctx context.Context,
	instanceID string,
	bindingID string,
) (domain.GetBindingSpec, error) {
	b.logger.Debug("get-binding-start", lager.Data{
		"instance-id": instanceID,
		"binding-id":  bindingID,
	})

	providerCtx, cancelFunc := context.WithTimeout(ctx, b.config.API.ContextTimeout())
	defer cancelFunc()

	data := provider.GetBindData{
		InstanceID: instanceID,
		BindingID:  bindingID,
	}

	binder, ok := b.AsyncBinderImplemented()
	if !ok {
		return domain.GetBindingSpec{}, ErrNotImplemented
	}
	res, err := binder.GetBinding(providerCtx, data)
	if err != nil {
		return domain.GetBindingSpec{}, err
	}
	if res == nil {
		return domain.GetBindingSpec{}, ErrNilResponse
	}

	b.logger.Debug("get-binding-success", lager.Data{
		"instance-id": instanceID,
		"binding-id":  bindingID,
	})

	return *res, nil
}

func (b *Broker) Update(
	ctx context.Context,
	instanceID string,
	details domain.UpdateDetails,
	asyncAllowed bool,
) (domain.UpdateServiceSpec, error) {
	b.logger.Debug("update-start", lager.Data{
		"instance-id":   instanceID,
		"details":       details,
		"async-allowed": asyncAllowed,
	})

	if !asyncAllowed {
		return domain.UpdateServiceSpec{}, apiresponses.ErrAsyncRequired
	}

	service, err := findServiceByID(b.config.Catalog, details.ServiceID)
	if err != nil {
		return domain.UpdateServiceSpec{}, err
	}

	if !service.PlanUpdatable && details.PlanID != details.PreviousValues.PlanID {
		return domain.UpdateServiceSpec{}, apiresponses.ErrPlanChangeNotSupported
	}

	plan, err := findPlanByID(service, details.PlanID)
	if err != nil {
		return domain.UpdateServiceSpec{}, err
	}

	providerCtx, cancelFunc := context.WithTimeout(ctx, b.config.API.ContextTimeout())
	defer cancelFunc()

	lock, err := b.ObtainServiceLock(providerCtx, instanceID, locketMaxTTL)
	if err != nil {
		return domain.UpdateServiceSpec{}, err
	}
	defer b.ReleaseServiceLock(providerCtx, lock)

	updateData := provider.UpdateData{
		InstanceID: instanceID,
		Details:    details,
		Service:    service,
		Plan:       plan,
	}

	updater, ok := b.UpdaterImplemented()
	if !ok {
		return domain.UpdateServiceSpec{}, ErrNotImplemented
	}
	res, err := updater.Update(providerCtx, updateData)
	if err != nil {
		return domain.UpdateServiceSpec{}, err
	}
	if res == nil {
		return domain.UpdateServiceSpec{}, ErrNilResponse
	}

	b.logger.Debug("update-success", lager.Data{
		"instance-id": instanceID,
		"details":     details,
		"is-async":    res.IsAsync,
	})

	return *res, nil
}

func (b *Broker) LastOperation(
	ctx context.Context,
	instanceID string,
	pollDetails domain.PollDetails,
) (domain.LastOperation, error) {
	b.logger.Debug("last-operation-start", lager.Data{
		"instance-id":  instanceID,
		"poll-details": pollDetails,
	})

	providerCtx, cancelFunc := context.WithTimeout(ctx, b.config.API.ContextTimeout())
	defer cancelFunc()

	lastOperationData := provider.LastOperationData{
		InstanceID:  instanceID,
		PollDetails: pollDetails,
	}

	provisioner, ok := b.AsyncProvisionerImplemented()
	if !ok {
		return domain.LastOperation{}, ErrNotImplemented
	}
	res, err := provisioner.LastOperation(providerCtx, lastOperationData)
	if err != nil {
		return domain.LastOperation{}, err
	}
	if res == nil {
		return domain.LastOperation{}, ErrNilResponse
	}

	b.logger.Debug("last-operation-success", lager.Data{
		"instance-id":  instanceID,
		"poll-details": pollDetails,
	})

	return *res, nil
}

func (b *Broker) LastBindingOperation(
	ctx context.Context,
	instanceID string,
	bindingID string,
	pollDetails domain.PollDetails,
) (domain.LastOperation, error) {
	b.logger.Debug("last-binding-operation-start", lager.Data{
		"instance-id":  instanceID,
		"binding-id":   bindingID,
		"poll-details": pollDetails,
	})

	providerCtx, cancelFunc := context.WithTimeout(ctx, b.config.API.ContextTimeout())
	defer cancelFunc()

	lastOperationData := provider.LastBindingOperationData{
		InstanceID:  instanceID,
		BindingID:   bindingID,
		PollDetails: pollDetails,
	}

	binder, ok := b.AsyncBinderImplemented()
	if !ok {
		return domain.LastOperation{}, ErrNotImplemented
	}
	res, err := binder.LastBindingOperation(providerCtx, lastOperationData)
	if err != nil {
		return domain.LastOperation{}, err
	}
	if res == nil {
		return domain.LastOperation{}, ErrNilResponse
	}

	b.logger.Debug("last-binding-operation-success", lager.Data{
		"instance-id":  instanceID,
		"binding-id":   bindingID,
		"poll-details": pollDetails,
	})

	return *res, nil
}

func (b *Broker) GetInstance(ctx context.Context, instanceID string) (domain.GetInstanceDetailsSpec, error) {
	return domain.GetInstanceDetailsSpec{}, errors.New("not implemented")
}

func (b *Broker) ObtainServiceLock(
	ctx context.Context,
	serviceName string,
	locketMaxTTL int,
) (ServiceLock, error) {
	// A Locket owner is the unique identifier of who currently owns this lock
	// Therefore each operation on the broker should be a unique owner
	// We generate a new UUID V4 for this

	lock := ServiceLock{
		ServiceName: serviceName,
		Key:         fmt.Sprintf("broker/%s", serviceName),
		Owner:       fmt.Sprintf("broker/%s", uuid.NewV4().String()),
	}

	lsession := b.logger.Session("obtain-lock-on-service", lager.Data{
		"service-name": serviceName,
		"lock":         lock,
	})

	lsession.Debug("begin")

	var err error
	for attempts := 0; attempts <= locketMaxTTL; attempts++ {
		_, err = b.LocketClient.Lock(
			ctx,
			&locket_models.LockRequest{
				Resource: &locket_models.Resource{
					Key:      lock.Key,
					Owner:    lock.Owner,
					TypeCode: locket_models.LOCK,
				},
				TtlInSeconds: int64(locketMaxTTL),
			},
		)

		if err == nil {
			break
		}

		// We should check for an acceptable error here, but in practice there are
		// many errors from grpc/locket/sqldb we should just try 15 times

		delay := DefaultLocketRetryInterval
		if b.config.API.Locket.RetryInterval > 0 {
			delay = b.config.API.Locket.RetryInterval
		}
		time.Sleep(delay)
	}

	if err != nil {
		lsession.Error("error", err)
		return lock, err
	}

	lsession.Debug("success")
	return lock, nil
}

func (b *Broker) ReleaseServiceLock(
	ctx context.Context,
	lock ServiceLock,
) {
	lsession := b.logger.Session("release-lock-on-service", lager.Data{
		"lock": lock,
	})

	lsession.Debug("begin")

	_, err := b.LocketClient.Release(
		ctx,
		&locket_models.ReleaseRequest{
			Resource: &locket_models.Resource{
				Key:      lock.Key,
				Owner:    lock.Owner,
				TypeCode: locket_models.LOCK,
			},
		},
	)

	if err != nil {
		lsession.Error("error", err)
	}
}
