package resourcequota

import (
	"encoding/json"

	"github.com/hashicorp/go-multierror"
	"github.com/shopspring/decimal"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/harvester/harvester/pkg/util"
)

type calculator struct {
	ResourceQuota              Quotas
	MaintenanceQuotaPercentage MaintenanceQuotaPercentage

	AvailableQuota   *Quotas
	MaintenanceQuota *Quotas
}

func newCalculatorByAnnotations(annotations map[string]string) (*calculator, error) {
	resourceQuotaStr, ok1 := annotations[util.AnnotationResourceQuota]
	maintenanceQuotaStr, ok2 := annotations[util.AnnotationMaintenanceQuota]
	if !ok1 || !ok2 || resourceQuotaStr == "" || maintenanceQuotaStr == "" {
		return nil, nil
	}

	return newCalculator(maintenanceQuotaStr, resourceQuotaStr)
}

func newCalculator(maintenanceJson, resourceQuotaJson string) (*calculator, error) {
	maintenanceQuota, err := parseMaintenanceQuota(maintenanceJson)
	if err != nil {
		return nil, err
	}
	resourceQuota, err := parseResourceQuota(resourceQuotaJson)
	if err != nil {
		return nil, err
	}
	return &calculator{
		MaintenanceQuotaPercentage: *maintenanceQuota,
		ResourceQuota:              *resourceQuota,
	}, nil
}

func (c *calculator) calculateResources() (*Quotas, *Quotas, error) {
	if err := c.calculateMaintenanceQuota(); err != nil {
		return nil, nil, err
	}

	if err := c.calculateAvailableQuota(); err != nil {
		return nil, nil, err
	}

	return c.AvailableQuota, c.MaintenanceQuota, nil
}

func (c *calculator) calculateAvailableQuota() error {
	if c.MaintenanceQuota == nil {
		if err := c.calculateMaintenanceQuota(); err != nil {
			return err
		}
	}

	limitsCPU := c.ResourceQuota.Limit.LimitsCPU.DeepCopy()
	limitsCPU.Sub(c.MaintenanceQuota.Limit.LimitsCPU)

	limitsMemory := c.ResourceQuota.Limit.LimitsMemory.DeepCopy()
	limitsMemory.Sub(c.MaintenanceQuota.Limit.LimitsMemory)

	c.AvailableQuota = &Quotas{
		Limit: QuotaLimit{
			LimitsCPU:    limitsCPU,
			LimitsMemory: limitsMemory,
		},
	}
	return nil
}

func (c *calculator) calculateMaintenanceQuota() error {
	var errCPU, errMemory error

	c.MaintenanceQuota = &Quotas{}
	c.MaintenanceQuota.Limit.LimitsCPU, errCPU = c.calculateMaintenanceResources(
		c.ResourceQuota.Limit.LimitsCPU,
		c.MaintenanceQuotaPercentage.Limit.LimitsCPUPercent)

	c.MaintenanceQuota.Limit.LimitsMemory, errMemory = c.calculateMaintenanceResources(
		c.ResourceQuota.Limit.LimitsMemory,
		c.MaintenanceQuotaPercentage.Limit.LimitsMemoryPercent)

	return multierror.Append(errCPU, errMemory).ErrorOrNil()
}

func (c *calculator) calculateMaintenanceResources(quota resource.Quantity, percent int64) (maintenanceAvail resource.Quantity, err error) {
	if percent < 1 {
		return resource.Quantity{}, nil
	}
	maintenanceQty, err := div(quota, percent)
	if err != nil {
		return resource.Quantity{}, err
	}
	return maintenanceQty, err
}

func parseMaintenanceQuota(maintenanceJson string) (*MaintenanceQuotaPercentage, error) {
	maintenanceQuota := &MaintenanceQuotaPercentage{}
	err := json.Unmarshal([]byte(maintenanceJson), maintenanceQuota)
	if err != nil {
		return nil, err
	}
	return maintenanceQuota, nil
}

func parseResourceQuota(resourceQuotaJson string) (*Quotas, error) {
	resourceQuota := &Quotas{}
	err := json.Unmarshal([]byte(resourceQuotaJson), resourceQuota)
	if err != nil {
		return nil, err
	}
	return resourceQuota, nil
}

// calculates the amount of resources that can be
// reserved for maintenance. The maintenance ratio is a percentage of
// the total available resources in the cluster.
func div(namespaceQuota resource.Quantity, percent int64) (resource.Quantity, error) {
	ratio := decimal.NewFromInt(percent).Div(decimal.NewFromInt(100))

	// Calculate the maintenance available resources
	maintQty := *resource.NewMilliQuantity(
		decimal.NewFromInt(namespaceQuota.MilliValue()).
			Mul(ratio).
			IntPart(),
		resource.DecimalSI)

	return maintQty, nil
}
