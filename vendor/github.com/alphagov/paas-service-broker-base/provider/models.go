package provider

import (
	"github.com/pivotal-cf/brokerapi/domain"
)

type ProvisionData struct {
	InstanceID string
	Details    domain.ProvisionDetails
	Service    domain.Service
	Plan       domain.ServicePlan
}

type DeprovisionData struct {
	InstanceID string
	Details    domain.DeprovisionDetails
	Service    domain.Service
	Plan       domain.ServicePlan
}

type BindData struct {
	InstanceID   string
	BindingID    string
	Details      domain.BindDetails
	AsyncAllowed bool
}

type UnbindData struct {
	InstanceID   string
	BindingID    string
	Details      domain.UnbindDetails
	AsyncAllowed bool
}

type UpdateData struct {
	InstanceID string
	Details    domain.UpdateDetails
	Service    domain.Service
	Plan       domain.ServicePlan
}

type LastOperationData struct {
	InstanceID  string
	PollDetails domain.PollDetails
}

type LastBindingOperationData struct {
	InstanceID string
	BindingID string
	PollDetails domain.PollDetails
}

type GetBindData struct {
	InstanceID string
	BindingID  string
}
