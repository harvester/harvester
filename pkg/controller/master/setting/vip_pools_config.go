package setting

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const KubevipConfigmapName = "kubevip"

func (h *Handler) syncVipPoolsConfig(setting *harvesterv1.Setting) error {
	pools := map[string]string{}
	err := json.Unmarshal([]byte(setting.Value), &pools)
	if err != nil {
		return err
	}

	poolsData := make(map[string]string, len(pools))
	for ns, pool := range pools {
		var prefix string
		if strings.Contains(pool, "-") {
			prefix = "range"
		} else {
			prefix = "cidr"
		}
		k := fmt.Sprintf("%s-%s", prefix, ns)
		poolsData[k] = pool
	}

	vipConfigmap, err := h.configmapCache.Get(util.KubeSystemNamespace, KubevipConfigmapName)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if errors.IsNotFound(err) {
		cf := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      KubevipConfigmapName,
				Namespace: util.KubeSystemNamespace,
			},
			Data: poolsData,
		}
		if _, err := h.configmaps.Create(cf); err != nil {
			return err
		}
	} else {
		vipConfigmapCpy := vipConfigmap.DeepCopy()
		vipConfigmapCpy.Data = poolsData
		if _, err := h.configmaps.Update(vipConfigmapCpy); err != nil {
			return err
		}

	}

	return nil
}

func ValidateCIDRs(pools map[string]string) error {
	for ns, pool := range pools {
		if strings.Contains(pool, "-") && strings.Contains(pool, "/") {
			return fmt.Errorf("invalid Pool value %s of %s, error: IP Range and CIDR cannot be used together", pool, ns)
		}
		cidrOrIPRanges := strings.Split(pool, ",")
		for _, cidrOrIPRange := range cidrOrIPRanges {
			if strings.HasPrefix(cidrOrIPRange, "-") || strings.HasSuffix(cidrOrIPRange, "-") {
				return fmt.Errorf("invalid IP Range value %s of %s", pool, ns)
			}
			ipRange := strings.Split(cidrOrIPRange, "-")
			switch len(ipRange) {
			case 1:
				if _, _, err := net.ParseCIDR(cidrOrIPRange); err != nil {
					return fmt.Errorf("invalid CIDR value %s of %s, error: %s", pool, ns, err.Error())
				}
			case 2:
				for _, ipAddr := range ipRange {
					if ip := net.ParseIP(ipAddr); ip == nil || ip.To4() == nil {
						return fmt.Errorf("invalid IP value %s of %s", pool, ns)
					}
				}
			default:
				return fmt.Errorf("invalid IP Range value %s of %s", pool, ns)
			}
		}
	}
	return nil
}
