package app

import (
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// === 网络纯函数（无状态） ===

// GetLANIP 返回供局域网内其它设备访问的「本机 IP」。
// 采用枚举网卡、筛选真实局域网 IPv4 的方式，避免旧实现用「UDP 连公网取出口 IP」
// 在开启 VPN 时被默认路由带偏到隧道口（如 198.18.0.1）的问题。
func GetLANIP() string {
	if cands := lanCandidates(); len(cands) > 0 {
		return cands[0]
	}
	// 兜底：极端环境（无可用物理网卡）下仍用出口 IP 探测
	if ip := udpEgressIP(); ip != "" {
		return ip
	}
	return "127.0.0.1"
}

// udpEgressIP 旧实现的兜底：UDP "连接" 公网地址（不会真的发包）读本地绑定 IP。
func udpEgressIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return ""
	}
	return addr.IP.String()
}

// lanCandidates 枚举所有网卡，筛出适合做局域网访问地址的 IPv4 候选，按优先级排序：
//  1. 物理网卡上的 RFC1918 私网地址（10/8、172.16/12、192.168/16）——局域网首选；
//  2. 其它全局单播地址（公网 IP）——兜底；
//  3. CGNAT（100.64/10，Tailscale / WireGuard 等）——仅在无前两者时兜底。
//
// 跳过：回环、未启用、链路本地（169.254/16、fe80::）、基准测试网段（198.18/15）、
// 以及隧道 / 虚拟网卡（VPN、容器、虚拟机网桥等）——这些地址局域网内其它设备通常无法直连。
func lanCandidates() []string {
	type cand struct {
		ip   net.IP
		prio int
	}
	var cands []cand
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range ifaces {
		// 只取已启用且非回环的网卡
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		// 跳过隧道 / 虚拟网卡（VPN、容器、虚拟机网桥等）：局域网其它设备直连不到
		if isVirtualIface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil {
				continue // 只取 IPv4（局域网文件互传场景 IPv4 足够）
			}
			prio, ok := lanPriority(ip)
			if !ok {
				continue
			}
			cands = append(cands, cand{ip: ip, prio: prio})
		}
	}
	if len(cands) == 0 {
		return nil
	}
	sort.SliceStable(cands, func(i, j int) bool {
		return cands[i].prio > cands[j].prio
	})
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.ip.String())
	}
	return out
}

// lanPriority 给候选 IPv4 打分：(优先级, 是否采纳)。
// 优先 RFC1918 私网，其次公网，最后 CGNAT；跳过大链路本地 / 基准测试网段。
func lanPriority(ip net.IP) (int, bool) {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return 0, false
	}
	// 基准测试网段 198.18.0.0/15（RFC2544）：VPN 隧道口常见伪地址，局域网不可达
	if benchmark19818.Contains(ip) {
		return 0, false
	}
	// RFC1918 私网：局域网首选
	if isRFC1918(ip) {
		return 3, true
	}
	// CGNAT 100.64.0.0/10（Tailscale / WireGuard 等）：仅在无私网时兜底
	if cgnat10064.Contains(ip) {
		return 1, true
	}
	// 其余全局单播（公网）：兜底
	if ip.IsGlobalUnicast() {
		return 2, true
	}
	return 0, false
}

// isVirtualIface 判断是否为隧道 / 虚拟网卡（VPN、容器、虚拟机网桥等）。
func isVirtualIface(name string) bool {
	n := strings.ToLower(name)
	prefixes := []string{"lo", "utun", "tun", "tap", "ppp", "ipsec", "wg", "vpn", "zt", "tailscale", "fl0", "awdl", "llw", "p2p", "anpi"}
	for _, p := range prefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	contains := []string{"vboxnet", "vmnet", "docker", "bridge", "veth", "ovpn"}
	for _, c := range contains {
		if strings.Contains(n, c) {
			return true
		}
	}
	return false
}

// RFC1918 / CGNAT / 基准测试网段，用于 lanPriority 判定。
var (
	benchmark19818 = &net.IPNet{IP: net.IPv4(198, 18, 0, 0), Mask: net.CIDRMask(15, 32)}
	cgnat10064     = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}
)

// isRFC1918 判断是否为 RFC1918 私网地址（10/8、172.16/12、192.168/16）。
func isRFC1918(ip net.IP) bool {
	for _, n := range []*net.IPNet{
		{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
		{IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)},
		{IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)},
	} {
		if n.Contains(ip) {
			return true
		}
	}
	return false
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

// deviceID 取请求携带的设备 ID（前端首次访问时生成 UUID 存 localStorage，
// 之后所有接口请求头携带 Deviceid）。用于删除归属判定（IP 变化后的兜底，
// 替代原 UA 设备签名）。
func deviceID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("Deviceid"))
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
