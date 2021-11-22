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
		k := fmt.Sprintf("cidr-%s", ns)
		poolsData[k] = pool
	}

	vipConfigmap, err := h.configmapCache.Get(util.KubeSystemNamespace, KubevipConfigmapName)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if vipConfigmap == nil {
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
	for ns, v := range pools {
		cidrs := strings.Split(v, ",")
		for _, cidr := range cidrs {
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				return fmt.Errorf("invalid CIDR value %s of %s, error: %s", v, ns, err.Error())
			}
		}
	}
	return nil
}
