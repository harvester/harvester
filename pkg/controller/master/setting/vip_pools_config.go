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
					if ip := net.ParseIP(ipAddr); ip == nil {
						return fmt.Errorf("invalid IP value %s of %s", pool, ns)
					}
				}
				if err := validateIPRangeSameFamily(ipRange[0], ipRange[1], pool, ns); err != nil {
					return err
				}
			default:
				return fmt.Errorf("invalid IP Range value %s of %s", pool, ns)
			}
		}
		// TODO(ipv6): remove once dual-stack ordering is no longer required
		if err := validateIPv4BeforeIPv6(cidrOrIPRanges, pool, ns); err != nil {
			return err
		}
	}
	return nil
}

// validateIPRangeSameFamily rejects a start-end range where one endpoint is
// IPv4 and the other is IPv6.
func validateIPRangeSameFamily(start, end, pool, ns string) error {
	a, b := net.ParseIP(start), net.ParseIP(end)
	if (a.To4() == nil) != (b.To4() == nil) {
		return fmt.Errorf("invalid IP Range value %s of %s: cannot mix IPv4 and IPv6 addresses", pool, ns)
	}
	return nil
}

// validateIPv4BeforeIPv6 enforces that all IPv4 entries precede all IPv6
// entries within a comma-separated pool value.
//
// TODO(ipv6): remove this function once the installer and UI can express
// dual-stack pools without an ordering constraint.
func validateIPv4BeforeIPv6(cidrOrIPRanges []string, pool, ns string) error {
	seenIPv6 := false
	for _, entry := range cidrOrIPRanges {
		ipRange := strings.Split(entry, "-")
		var representative net.IP
		if len(ipRange) == 2 {
			representative = net.ParseIP(ipRange[0])
		} else {
			var err error
			representative, _, err = net.ParseCIDR(entry)
			if err != nil {
				return fmt.Errorf("invalid CIDR value %s of %s, error: %s", entry, ns, err.Error())
			}
		}
		if representative == nil {
			continue
		}
		isIPv6 := representative.To4() == nil
		if seenIPv6 && !isIPv6 {
			return fmt.Errorf("invalid Pool value %s of %s: IPv4 entries must precede IPv6 entries", pool, ns)
		}
		seenIPv6 = seenIPv6 || isIPv6
	}
	return nil
}
