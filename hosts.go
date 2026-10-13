package tdx

import (
	"context"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/injoyai/conv"
	"github.com/injoyai/logs"
)

// Addrs 是包含端口的 TCP 地址列表，可直接传入接收 []string 的拨号函数。
type Addrs []string

var (

	// Hosts 标准行情服务器地址，格式为 IP:端口(2024-11-30测试通过)
	Hosts = func() Addrs {
		lenSH := len(SHHosts)
		lenBJ := len(BJHosts)
		lenGZ := len(GZHosts)
		lenWH := len(WHHosts)

		ls := make(Addrs, lenSH+lenBJ+lenGZ+lenWH)
		copy(ls[:lenSH], SHHosts)
		copy(ls[lenSH:lenSH+lenBJ], BJHosts)
		copy(ls[lenSH+lenBJ:lenSH+lenBJ+lenGZ], GZHosts)
		copy(ls[lenSH+lenBJ+lenGZ:lenSH+lenBJ+lenGZ+lenWH], WHHosts)
		return ls
	}()

	// SHHosts 上海服务器地址
	SHHosts = Addrs{
		"124.71.187.122:7709",  //华为
		"122.51.120.217:7709",  //腾讯
		"111.229.247.189:7709", //腾讯
		"122.51.232.182:7709",  //腾讯
		"118.25.98.114:7709",   //腾讯
		"124.70.199.56:7709",   //华为
		"121.36.225.169:7709",  //华为
		"123.60.70.228:7709",   //华为
		"123.60.73.44:7709",    //华为
		"124.70.133.119:7709",  //华为
		"124.71.187.72:7709",   //华为
		"123.60.84.66:7709",    //华为
		"124.223.163.242:7709", //腾讯云
		"150.158.160.2:7709",   //腾讯云
		"101.35.121.35:7709",   //腾讯云
		"111.231.113.208:7709", //腾讯云
	}

	// BJHosts 北京服务器地址
	BJHosts = Addrs{
		"62.234.50.143:7709",   //腾讯云
		"81.70.151.186:7709",   //腾讯云
		"101.42.240.54:7709",   //腾讯云
		"101.43.159.194:7709",  //腾讯云
		"120.53.8.251:7709",    //腾讯云
		"152.136.191.169:7709", //腾讯云
		"49.232.15.141:7709",   //腾讯云
		"82.156.174.84:7709",   //腾讯云
		"101.42.164.241:7709",  //腾讯云

	}

	// GZHosts 广州服务器地址,客户端上可能显示深圳
	GZHosts = Addrs{
		"124.71.9.153:7709",    //华为
		"116.205.163.254:7709", //华为
		"116.205.171.132:7709", //华为
		"116.205.183.150:7709", //华为
		"111.230.186.52:7709",  //腾讯
		"110.41.2.72:7709",     //华为
		"110.41.147.114:7709",  //华为,这个客户端显示深圳线路1,IP查询是广州的
		"101.33.225.16:7709",   //腾讯云
		"175.178.112.197:7709", //腾讯云
		"175.178.128.227:7709", //腾讯云
		"43.139.95.83:7709",    //腾讯云
		"159.75.29.111:7709",   //腾讯云
		"43.139.18.171:7709",   //腾讯云
		"81.71.32.47:7709",     //腾讯云
		"129.204.230.128:7709", //腾讯云
	}

	// WHHosts 武汉服务器地址
	WHHosts = Addrs{
		"119.97.185.59:7709", //电信
	}
)

// 扩展行情(TdxExHq)服务器,端口 7727,地址取自通达信 connect.cfg [DSHOST] 段。
// 按地域拆分子表，地址统一使用 IP:端口。
const ExPort = "7727"

var (

	// ExHosts 扩展行情服务器地址，格式为 IP:端口(connect.cfg [DSHOST])
	// 广州段在前:2026-06 实测仅 116.205.143.214(广州双线1)完全可用,置首位使
	// range-dial 默认优先命中;其余多为连接超时/受限节点,可用 ExHosts.Sort() 重排。
	ExHosts = func() Addrs {
		lenGZ := len(ExGZHosts)
		lenSH := len(ExSHHosts)
		lenBJ := len(ExBJHosts)

		ls := make(Addrs, lenGZ+lenSH+lenBJ)
		copy(ls[:lenGZ], ExGZHosts)
		copy(ls[lenGZ:lenGZ+lenSH], ExSHHosts)
		copy(ls[lenGZ+lenSH:lenGZ+lenSH+lenBJ], ExBJHosts)
		return ls
	}()

	// ExSHHosts 扩展行情上海服务器地址
	ExSHHosts = Addrs{
		"175.24.47.69:7727",   //扩展市场上海双线7
		"150.158.9.199:7727",  //扩展市场上海双线1
		"150.158.20.127:7727", //扩展市场上海双线2
		"49.235.119.116:7727", //扩展市场上海双线3
		"49.234.13.160:7727",  //扩展市场上海双线4
		"123.60.173.210:7727", //扩展市场上海双线5
		"118.89.69.202:7727",  //扩展市场上海双线6
	}

	// ExBJHosts 扩展行情北京服务器地址
	ExBJHosts = Addrs{
		"112.74.214.43:7727",  //扩展市场北京双线1
		"120.25.218.6:7727",   //扩展市场北京双线2
		"43.139.173.246:7727", //扩展市场北京双线3
		"159.75.90.107:7727",  //扩展市场北京双线4
		"106.52.170.195:7727", //扩展市场北京双线5
		"139.9.191.175:7727",  //扩展市场北京双线6
	}

	// ExGZHosts 扩展行情广州服务器地址
	ExGZHosts = Addrs{
		"116.205.143.214:7727", //扩展市场广州双线1
		"124.71.223.19:7727",   //扩展市场广州双线2
		"113.45.175.47:7727",   //扩展市场广州双线4
	}
)

// Sort 按 TCP 连接耗时筛选排序地址列表，地址须包含端口。
// 返回新列表，不修改输入或默认列表；空输入不使用默认地址。
// timeout 是整轮测速的时间上限，默认一秒；没有可用地址时返回空列表。
func (a Addrs) Sort(timeout ...time.Duration) Addrs {
	ctx, cancel := context.WithTimeout(context.Background(), conv.Default(time.Second, timeout...))
	defer cancel()
	type result struct {
		host    string
		elapsed time.Duration
	}
	results := make(chan result, len(a))
	for _, host := range a {
		go func(host string) {
			now := time.Now()
			c, err := (&net.Dialer{}).DialContext(ctx, "tcp", host)
			r := result{}
			if err != nil {
				logs.Err(err)
			} else {
				r = result{host: host, elapsed: time.Since(now)}
				c.Close()
				logs.Debugf("Host: %-21s  Speed: %s\n", host, r.elapsed)
			}
			results <- r
		}(host)
	}

	var available []result
	for range a {
		if r := <-results; r.host != "" {
			available = append(available, r)
		}
	}
	sort.Slice(available, func(i, j int) bool { return available[i].elapsed < available[j].elapsed })
	var addrs Addrs
	for _, r := range available {
		addrs = append(addrs, r.host)
	}
	return addrs
}

func withPort(addr, port string) string {
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}
	// 裸 IPv6（含 zone）本身带冒号，不能据此判断已有端口。
	host := addr
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	ip, _, _ := strings.Cut(host, "%")
	if net.ParseIP(ip) != nil {
		return net.JoinHostPort(host, port)
	}
	if !strings.Contains(addr, ":") {
		return net.JoinHostPort(addr, port)
	}
	return addr
}

func hostAddrs(hosts, defaults []string, port string) []string {
	if len(hosts) == 0 {
		hosts = defaults
	}
	addrs := make([]string, len(hosts))
	for i, host := range hosts {
		addrs[i] = withPort(host, port)
	}
	return addrs
}
