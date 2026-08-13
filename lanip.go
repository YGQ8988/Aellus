// 局域网 IP 获取：UDP socket 连接法，不真正发包，跨平台无系统命令依赖。
package main

import "net"

// getLanIP 返回本机出局网卡的 IPv4 地址。
//
// 原理：对 8.8.8.8:80 发起 UDP "连接"——UDP 无握手，不会真正发包，
// 仅让操作系统按路由表选定出口网卡并绑定本地地址，随即读出即可。
// 任何错误（无网卡/离线）回退为占位串。
func getLanIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "<本机IP>"
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "<本机IP>"
	}
	return addr.IP.String()
}
