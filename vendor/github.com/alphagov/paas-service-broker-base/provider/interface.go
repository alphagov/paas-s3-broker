package provider

import (
	"context"

	"github.com/pivotal-cf/brokerapi/domain"
)

type Provider interface{}

type Updater interface {
	Update(context.Context, UpdateData) (res *domain.UpdateServiceSpec, err error)
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -o fakes/fake_provisioner.go . Provisioner
type Provisioner interface {
	Provision(context.Context, ProvisionData) (res *domain.ProvisionedServiceSpec, err error)
	Deprovision(context.Context, DeprovisionData) (res *domain.DeprovisionServiceSpec, err error)
}

type AsyncProvisioner interface {
	Provisioner
	LastOperation(context.Context, LastOperationData) (state *domain.LastOperation, err error)
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -o fakes/fake_binder.go . Binder
type Binder interface {
	Bind(context.Context, BindData) (binding *domain.Binding, err error)
	Unbind(context.Context, UnbindData) (unbinding *domain.UnbindSpec, err error)
}

type AsyncBinder interface {
	Binder
	LastBindingOperation(context.Context, LastBindingOperationData) (state *domain.LastOperation, err error)
	GetBinding(context.Context, GetBindData) (state *domain.GetBindingSpec, err error)
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -o fakes/fake_async_provider.go . AsyncProvider
type AsyncProvider interface {
	AsyncProvisioner
	AsyncBinder
	Updater
}
