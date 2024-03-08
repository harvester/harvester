package util

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	HTTPClientErrorPrefixTemplate = "resp.StatusCode(%d) != http.StatusOK(200)"

	EnvPodIP = "POD_IP"

	StorageNetworkInterface = "lhnet1"
)

func GetHTTPClientErrorPrefix(stateCode int) string {
	return fmt.Sprintf(HTTPClientErrorPrefixTemplate, stateCode)
}

func IsHTTPClientErrorNotFound(inputErr error) bool {
	return inputErr != nil && strings.Contains(inputErr.Error(), GetHTTPClientErrorPrefix(http.StatusNotFound))
}

func DetectHTTPServerAvailability(url string, waitIntervalInSecond int, shouldAvailable bool) bool {
	cli := http.Client{
		Timeout: time.Second,
	}

	endTime := time.Now().Add(time.Duration(waitIntervalInSecond) * time.Second)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		<-ticker.C

		resp, err := cli.Get(url)
		if resp != nil && resp.Body != nil {
			if err := resp.Body.Close(); err != nil {
				logrus.WithError(err).Error("failed to close the response body during the HTTP server detection")
			}
		}
		if err != nil && !shouldAvailable {
			return true
		}
		if err == nil && shouldAvailable {
			return true
		}
		if !time.Now().Before(endTime) {
			return false
		}
	}
}

func GetIPForPod() (ip string, err error) {
	var storageIP string
	if ip, err := GetLocalIPv4fromInterface(StorageNetworkInterface); err != nil {
		storageIP = os.Getenv(EnvPodIP)
		logrus.WithError(err).Debugf("failed to get IP from %v interface, fallback to use the default pod IP %v", StorageNetworkInterface, storageIP)
	} else {
		storageIP = ip
	}
	if storageIP == "" {
		return "", fmt.Errorf("can't get a ip from either the specified interface or the environment variable")
	}
	return storageIP, nil
}

func GetLocalIPv4fromInterface(name string) (ip string, err error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", err
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", errors.Wrapf(err, "interface %s doesn't have address\n", name)
	}

	var ipv4 net.IP
	for _, addr := range addrs {
		if ipv4 = addr.(*net.IPNet).IP.To4(); ipv4 != nil {
			break
		}
	}
	if ipv4 == nil {
		return "", errors.Errorf("interface %s don't have an IPv4 address\n", name)
	}

	return ipv4.String(), nil
}

func GetSyncServiceAddressWithPodIP(address string) (string, error) {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	podIP := os.Getenv(EnvPodIP)
	return net.JoinHostPort(podIP, port), nil
}
