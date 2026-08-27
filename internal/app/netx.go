package app

import (
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// === 网络纯函数（无状态） ===

// GetLANIP 获取本机局域网 IP。
// 技巧：用一个 UDP "连接" 到公网地址（不会真的发包），然后读取本地绑定的 IP，
// 这个 IP 就是当前用来上网的网卡 IP（通常是局域网 IP）。比遍历网卡更可靠。
func GetLANIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "127.0.0.1"
	}
	return addr.IP.String()
}

// ListenWithFallback 从 start 端口开始尝试监听，被占用就 +1 继续试，
// 直到找到一个空闲端口。返回监听器和实际用到的端口。
func ListenWithFallback(start int) (net.Listener, int) {
	for p := start; p < start+100; p++ {
		ln, err := net.Listen("tcp", ":"+strconv.Itoa(p))
		if err == nil {
			return ln, p
		}
	}
	log.Fatal("找不到可用端口（已尝试 " + strconv.Itoa(start) + " ~ " + strconv.Itoa(start+99) + "）")
	return nil, 0
}

// ListenStrict 严格监听指定端口，被占用直接 Fatal 退出，不尝试 +1。
// 用于飞牛等平台环境：平台已做端口管理（manifest service_port + 向导选择），
// 应用静默换端口会导致端口错位（平台认声明的端口，但应用实际监听在别处，
// 表现为「进程活着但声明的端口访问不到」）。因此必须严格监听声明的端口。
func ListenStrict(port int) (net.Listener, int) {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		log.Fatal("端口 " + strconv.Itoa(port) + " 被占用或无法监听：" + err.Error())
	}
	return ln, port
}

// clientIP 取请求客户端 IP（去端口），与记录归属时一致。
func clientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 && strings.Count(host, ":") == 1 {
		host = host[:i]
	}
	return host
}

// realIP 取真实客户端 IP：优先 X-Forwarded-For 首段（有反代时），否则取 RemoteAddr。
func realIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	return r.RemoteAddr
}

// remoteIP 从 RemoteAddr 解析对端 IP（去掉端口）。
// 注意：只信 TCP 对端地址，不读 X-Forwarded-For 等请求头——头可被局域网内
// 其他设备伪造，用它做"仅本机"判断会被绕过（归属判定另用 clientIP，不受影响）。
func remoteIP(r *http.Request) net.IP {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil
	}
	return net.ParseIP(h)
}

// isLocalIP 判断 IP 是否为本机：回环地址，或本机任意网卡上的地址。
func isLocalIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ipn net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ipn = v.IP
			case *net.IPAddr:
				ipn = v.IP
			}
			if ipn != nil && ipn.Equal(ip) {
				return true
			}
		}
	}
	return false
}

// isLocalRequest 判断请求是否来自本机（用于限制只有本机才能改设置/弹目录选择框）。
// 判定依据（满足其一即本机）：
//  1. Host 是 localhost / 127.0.0.1 / [::1]（loopback 访问）；
//  2. 真实客户端 IP（RemoteAddr）是本机自身的网卡地址——覆盖"本机用局域网 IP
//     访问"的情况（换浏览器/手动输入 IP 时 Host 是 192.168.x.x，但客户端仍是本机）。
//
// 局域网内其他设备即使伪造 Host 也过不了第 2 条（它的 IP 不是本机网卡地址）。
func isLocalRequest(r *http.Request) bool {
	host := r.Host
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") || strings.HasPrefix(host, "[::1]") {
		return true
	}
	return isLocalIP(remoteIP(r))
}
