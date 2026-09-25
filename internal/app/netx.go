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
// 采用枚举网卡、筛选真实局域网 IPv4 的方式，避免开启 VPN 时被默认路由带偏到隧道口（如 198.18.0.1）。
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

// udpEgressIP 兜底：UDP "连接" 公网地址（不会真的发包）读本地绑定 IP。
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

// isVirtualIface 判断是否为隧道 / 虚拟 / 网桥网卡（VPN、容器、虚拟机网络等）。
//
// 两个用途：
//  1. GetLANIP 选局域网地址时跳过它们（局域网其它设备直连不到）；
//  2. isLocalIP 判定「本机来源」时跳过它们（容器流量不算本机，见该函数注释）。
//
// 名单覆盖常见命名：Linux 网桥/Docker（docker0、br-<id>、veth*）、飞牛/群晖类网桥
// （vbr*、ovs*）、libvirt（virbr*）、K8s/容器运行时（cni*、flannel*、cali*、kube*）、
// VM（vmnet*、vboxnet*、vmbr*、vmenet*）、VPN/隧道（utun*、tun*、wg*、tailscale*、zt*）。
// 物理网卡名（en0、eth0、ens*、wlan0、bond0…）不受影响。
func isVirtualIface(name string) bool {
	n := strings.ToLower(name)
	prefixes := []string{
		"lo", "utun", "tun", "tap", "ppp", "ipsec", "wg", "vpn", "zt", "tailscale",
		"fl0", "awdl", "llw", "p2p", "anpi",
		// 容器 / 网桥 / 虚拟网络（Linux 为主，NAS 上常见）
		// 注意 br0 / br1 / lxdbr / podman 这类不带分隔符的常见网桥名也要覆盖，
		// 否则它们的地址会被 isLocalIP 误判为「本机」而授予管理权限。
		"br-", "br0", "br1", "vbr", "ovs", "virbr", "cni", "flannel", "cali", "kube",
		"vmenet", "vmbr", "lxdbr", "podman",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	contains := []string{"vboxnet", "vmnet", "docker", "bridge", "veth", "ovpn", "nerdctl", "lxcbr", "multipass"}
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

// deviceID 取请求携带的设备 ID（前端首次访问时生成 UUID 存 localStorage，
// 之后所有接口请求头携带 Deviceid）。用于设备名映射与访问日志记录。
func deviceID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("Deviceid"))
}

// 注意：不要从请求头（X-Forwarded-For / X-Real-IP 等）读取客户端 IP。
// 本应用没有任何反向代理，这些头只可能来自客户端伪造，用它记日志会让操作被栽赃到
// 别的 IP 上；需要客户端地址时一律用 remoteIP（只信 TCP 对端）。

// remoteIP 从 RemoteAddr 解析对端 IP（去掉端口）。
// 注意：只信 TCP 对端地址，不读 X-Forwarded-For 等请求头——头可被局域网内
// 其他设备伪造，用它做"仅本机"判断会被绕过。
func remoteIP(r *http.Request) net.IP {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil
	}
	return net.ParseIP(h)
}

// isLocalIP 判断 IP 是否为本机：回环地址，或本机【物理】网卡上的地址。
//
// 注意：虚拟 / 网桥 / 隧道网卡上的地址不算本机。容器（Docker/containerd）、虚拟机、
// VPN 的流量出到主机时，源地址就是主机网桥上那个网关地址（如 172.17.0.1、br-xxxx），
// 若把它当作「本机」，容器里的进程就能直接拿到删除 / 改保存目录权限——而它并不是
// 「运行应用的这台电脑上的用户」。回环地址仍然算本机（本机浏览器访问 127.0.0.1）。
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
		if isVirtualIface(iface.Name) {
			continue // 物理网卡之外的地址一律不算本机
		}
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

// isLocalRequest 判断请求是否来自本机（用于限制只有本机才能改设置/删文件）。
// 判定依据：只信 TCP 对端地址（RemoteAddr）——回环地址，或本机【物理】网卡地址
// （虚拟 / 网桥 / 隧道网卡上的地址不算，见 isLocalIP）。
//
// 为什么不看 Host：Host 属于请求头（客户端可任意伪造），局域网设备只要发
// 「Host: 127.0.0.1」就能冒充本机；而 TCP 源地址无法伪造，故本机判定只依据它。
func isLocalRequest(r *http.Request) bool {
	return isLocalIP(remoteIP(r))
}

// gatewayUser 返回「飞牛统一网关注入的用户身份头」的值；无则返回空串。
// 官方文档存在两种写法（X-Trim-Userid / X-Trim-Uid），两者都接受，
// 用户名（X-Trim-Username）作为兜底——它们只会出现在经网关转发的请求上：
// 裸端口入口已由 stripTrimHeaders 剥离任何客户端伪造的 X-Trim-*。
func gatewayUser(r *http.Request) string {
	for _, k := range []string{"X-Trim-Userid", "X-Trim-Uid", "X-Trim-Username"} {
		if v := strings.TrimSpace(r.Header.Get(k)); v != "" {
			return v
		}
	}
	return ""
}

// canManage 判断请求是否有「删除文件 / 修改保存目录」权限（前端按钮显隐与服务端强制一致）。
//
// 判定依据（满足其一即可）：
//  1. 请求带飞牛统一网关注入的身份头（见 gatewayUser）——只有经网关（已完成飞牛
//     登录态校验）转发的请求才有；裸端口入口已在 stripTrimHeaders 中剥离伪造的
//     X-Trim-*，因此局域网设备手动构造该头也无法冒充「来自已登录门户」。
//  2. 请求来自本机（isLocalRequest，仅依据 TCP 源地址）——桌面端「谁运行应用，
//     那台电脑就是管理员」；也兼容飞牛门户经 NAS 本机转发到应用的情形。
//
// 例外：飞牛构建下若统一网关已成功监听，则【不再接受第 2 条】——NAS 上的任意本机
// 进程、以及容器网桥地址（docker0 / vbr 等同样被 isLocalIP 视为本机）否则都能绕过
// 飞牛账号体系拿到管理权。网关未起来时仍回退到第 2 条，避免门户整体不可用。
//
// 局域网设备经 IP:端口 直连时两条都不满足，故只能浏览、上传、下载。
func (a *App) canManage(r *http.Request) bool {
	if gatewayUser(r) != "" {
		return true
	}
	if a.platform.EnforceAuthBoundary() && a.gatewayActive.Load() {
		return false
	}
	return isLocalRequest(r)
}
