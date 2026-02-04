package core

import (
	"fmt"
	"net"
)

// 获取没有绑定服务的端口
func GetNoPortExists() string {
	startPort := 1000 //1000以下的端口很多时间需要root权限才能使用
	for port := startPort; port < 65535; port++ {
		l, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			continue
		}
		l.Close()
		return fmt.Sprintf("%d", port)
	}

	return "0"
}
