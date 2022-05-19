package main

import (
	"bufio"
	"fmt"
	"github.com/go-ping/ping"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

var GDomains = []string{
	"github.com",
	"www.github.com",
	"github.global.ssl.fastly.net",
	"github.map.fastly.net",
	"github.githubassets.com",
	"github.io",
	"assets-cdn.github.com",
	"gist.github.com",
	"help.github.com",
	"api.github.com",
	"nodeload.github.com",
	"codeload.github.com",
	"raw.github.com",
	"documentcloud.github.com",
	"status.github.com",
	"training.github.com",
	"raw.githubusercontent.com",
	"gist.githubusercontent.com",
	"cloud.githubusercontent.com",
	"camo.githubusercontent.com",
	"avatars0.githubusercontent.com",
	"avatars1.githubusercontent.com",
	"avatars2.githubusercontent.com",
	"avatars3.githubusercontent.com",
	"avatars4.githubusercontent.com",
	"avatars5.githubusercontent.com",
	"avatars6.githubusercontent.com",
	"avatars7.githubusercontent.com",
	"avatars8.githubusercontent.com",
	"user-images.githubusercontent.com",
	"favicons.githubusercontent.com",
	"github-cloud.s3.amazonaws.com",
	"github-production-release-asset-2e65be.s3.amazonaws.com",
	"github-production-user-asset-6210df.s3.amazonaws.com",
	"github-production-repository-file-5c1aeb.s3.amazonaws.com",
	"alive.github.com",
	"guides.github.com",
	"docs.github.com",
}

var (
	wg                   sync.WaitGroup
	chanTask             chan string
	urlCount             int
	domainMultipleIPList []map[string][]string
	domainIPList         []map[string]string
	MIN                  = 0.00000001
)

func fetchContent(url string) string {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println("无法发送网络请求，请检查你的网络：", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.127 Safari/537.36")
	req.Header.Add("Referer", "https://github.com/")    // 必须添加此行，不然会报错 403
	req.Header.Add("accept-language", "zh-CN,zh;q=0.9") // 必须添加此行，不然会报错 403

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		fmt.Println("连接 ipaddress.com 异常：", err)
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("读取 ipaddress.com 返回的 HTML 数据失败：", err)
	}

	rsBody := string(body)
	return rsBody
}

func parseGithubIp(url string) {
	HTMLContent := fetchContent(url)
	re := regexp.MustCompile(`href="https://www.ipaddress.com/ipv4/(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)){3}">`)
	matchContent := re.FindAllString(HTMLContent, -1)
	var ipaddr []string
	for _, content := range matchContent {
		getIp := content[37 : len(content)-2]
		ipaddr = append(ipaddr, getIp)
	}

	if len(ipaddr) != 1 {
		// 解析到多个 ip 时，将其添加到 domainMultipleIPList 中，然后通过协程获取这几个ip中 avgRtt 值最小的 ip作为最终的 ip
		domainMultipleIP := make(map[string][]string)
		domainMultipleIP[url] = ipaddr
		domainMultipleIPList = append(domainMultipleIPList, domainMultipleIP)
	} else {
		// 只解析到一个 ip 的域名，直接将其添加到 domainIPList 列表中，这个列表里的数据即最终 url:ip 对应的数据
		domainIP := make(map[string]string)
		ipstr := ipaddr[0]
		fmt.Printf("%s 解析到的ip是： %v\n", url, ipstr)
		domainIP[url] = ipstr
		domainIPList = append(domainIPList, domainIP)
	}

	chanTask <- url
	wg.Done()
}

func getLowRttIp(url string, ips []string) {
	var ipRttList []map[string]float64
	var packetLossIPRttList []map[string]float64
	var rttiplist []map[float64]string
	var minRtt float64 = 10

	for _, ip := range ips {
		pinger, err := ping.NewPinger(ip)

		if err != nil {
			fmt.Println(err)
		}
		pinger.Count = 10
		pinger.Interval = time.Millisecond * 100
		pinger.Timeout = time.Second * 3 // 总耗时 3s
		//pinger.SetPrivileged(true)       // windows 上运行必须添加此行代码,Linux/Unix 上必须注释此行代码
		pinger.OnFinish = func(statistics *ping.Statistics) {
			// 没有丢包的 ip 与 rtt 关系列表，最后结果是: [map[185.199.108.133:0.20682429],map[185.199.110.133:0.209455546]]
			if statistics.PacketLoss == 0 {
				ipRtt := make(map[string]float64, 6)
				var floatAvgRtt = statistics.AvgRtt.Seconds()
				ipRtt[statistics.Addr] = floatAvgRtt // 这里没有用 rtt 作为 key 是担心 rtt 有相同的值,所以用 ip 作为 key 可以保证唯一性
				ipRttList = append(ipRttList, ipRtt)

			}
			// 出现丢包的 ip 与 rtt 关系列表
			if statistics.PacketLoss != 100 && statistics.PacketLoss > 0 {
				packetLossIpRtt := make(map[string]float64, 6)
				var pkgLossFloatAvgRtt = statistics.AvgRtt.Seconds()
				packetLossIpRtt[statistics.Addr] = pkgLossFloatAvgRtt
				packetLossIPRttList = append(packetLossIPRttList, packetLossIpRtt)
			}

			//stats := pinger.Statistics()
			//fmt.Println(stats)
		}

		err = pinger.Run()
		if err != nil {
			fmt.Println("error")
		}
	}

	//fmt.Printf("域名为 %s 中没有出现丢包的ip为： %v\n", url, ipRttList)
	//fmt.Printf("域名为 %s 中出现丢包的ip为： %v\n", url, packetLossIPRttList)

	// 当解析到的 ip 都出现丢包的时候，只能从 packetLossIPRttList 里的出现丢包的 ip 中选择值
	if len(ipRttList) == 0 {
		ipRttList = packetLossIPRttList
	}

	// 目的是获得最小的 avgRtt 值
	for _, list := range ipRttList {
		for _, rtt := range list {
			if rtt < minRtt {
				minRtt = rtt
			}

		}
	}

	//fmt.Printf("%s 已取得最小的 rtt 值，等待添加找到其匹配的 ip 值的代码：%v\n", url, minRtt)

	// 将原来的列表进行反转得到以 rtt 为 key 的新列表，目的是为了通过 value 获取 key 的值
	revorseList := make(map[float64]string)
	for _, value := range ipRttList {
		for key, values := range value {
			revorseList[values] = key
		}
	}
	rttiplist = append(rttiplist, revorseList)

	// 从反转后的列表中通过 avgRtt 值获取到对应的 Ip
	for _, allrttip := range rttiplist {
		for rtt, ip := range allrttip {
			if isEqual(rtt, minRtt) {
				fmt.Printf("%s 解析到%d个ip ，最终ip(取最小Avgrtt值)为： %s\n", url, len(ips), ip)
				realiprttlist := make(map[string]string)
				realiprttlist[url] = ip
				domainIPList = append(domainIPList, realiprttlist) // 添加到最终的 url:ip 列表中
			}
		}
	}

	wg.Done()
}

func checkTask() {
	var count int
	for {
		<-chanTask
		//url := <-chanTask
		//fmt.Printf("%s 完成了爬取任务\n", url)
		count++
		if count == urlCount {
			break
		}
	}
	wg.Done()
}

func isEqual(f1, f2 float64) bool {
	if f1 > f2 {
		return f1-f2 < MIN
	} else {
		return f2-f1 < MIN
	}
}

func fileDeduplication(line string) (isDuplication bool) {
	isDuplication = false
	for _, siteName := range GDomains {
		if strings.Contains(line, siteName) || strings.Contains(line, "更新") {
			isDuplication = true
		}
	}

	return isDuplication
}

func copyFile(srcFile, destFile string) (written int64, err error) {
	srcFileData, err := os.OpenFile(srcFile, os.O_RDWR, os.ModePerm)
	if err != nil {
		fmt.Println(err)
		return
	}

	destFileData, err := os.OpenFile(destFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	if err != nil {
		fmt.Println(err)
	}

	defer srcFileData.Close()
	defer destFileData.Close()

	return io.Copy(destFileData, srcFileData)
}

func checkOSPlatform() (isWindows bool, hostPath string) {
	isWindows = false
	if runtime.GOOS == "windows" {
		isWindows = true
		hostPath = "C:\\Windows\\System32\\drivers\\etc\\hosts"
		fmt.Printf("\n操作系统为Windows，其hosts文件路径为： %s\n", hostPath)
	} else {
		hostPath = "/etc/hosts"
		fmt.Printf("\n操作系统为Linux/Unix，其hosts文件路径为：%s\n", hostPath)
	}

	return isWindows, hostPath
}

func flushDNSCache(isWindows bool) {
	if isWindows {
		exec.Command("ipconfig", "/flushdns")
		fmt.Printf("\n系统缓存已更新！\n")
		fmt.Printf("按下 回车键 或 Ctrl+C 退出。")
		var pause int
		fmt.Scanln(&pause)
	} else {
		exec.Command("systemd-resolve", "--", "flush-caches")
		fmt.Printf("\n已更新系统缓存！\n")
	}
}

func updateHostsFile() {
	isWindows, hostPath := checkOSPlatform()

	fmt.Printf("\n正在更新系统hosts文件...请不要关闭此窗口!\n")

	// 备份文件
	hostsBack := hostPath + ".bak"
	_, err := copyFile(hostPath, hostsBack)
	if err != nil {
		fmt.Println("hosts 文件备份失败：", err)
	}

	// 文件去重与写入临时文件
	fileHost, err := os.OpenFile(hostPath, os.O_RDWR, os.ModePerm)
	if err != nil {
		fmt.Println(err)
	}

	tmpFile := hostPath + "tmp"
	tmpContent, err := os.OpenFile(tmpFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	if err != nil {
		fmt.Println(err)
	}

	defer fileHost.Close()
	defer tmpContent.Close()

	reader := bufio.NewReader(fileHost)
	for {
		line, err := reader.ReadString('\n')
		isDuplication := fileDeduplication(line)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println(err)
		}

		if !isDuplication {
			tmpContent.WriteString(line)
		}

	}

	today := time.Now().Format("2006-01-02 15:04:05")
	tmpContent.WriteString("# github相关域名的ip信息已于  " + string(today) + "  完成更新 \n")

	// 将解析到的域名与ip关系写到临时文件中
	for _, doaminip := range domainIPList {
		for domain, ip := range doaminip {
			realDomain := strings.Split(domain, "/")[4]
			tmpContent.WriteString(ip + "\t" + realDomain + "\n")
		}
	}

	// 更新 Hosts 文件
	_, err = copyFile(tmpFile, hostPath)
	if err != nil {
		fmt.Println(err)
	}

	flushDNSCache(isWindows)

}

func main() {
	start := time.Now()
	fmt.Println("正在解析网页获取最新的 Github IP，请稍后...")
	urlCount = len(GDomains)
	chanTask = make(chan string, urlCount)

	// 开始爬虫任务，获得 html 内容，并解析到 ip 信息
	for _, urls := range GDomains {
		wg.Add(1)
		url := "https://ipaddress.com/website/" + urls
		go parseGithubIp(url)
	}

	// 监控任务是否已完成
	wg.Add(1)
	go checkTask()

	wg.Wait()

	// 解析到多个ip时对其进行ping获取最小avgRtt值的ip
	for _, domainsIP := range domainMultipleIPList {
		wg.Add(1)
		for domain, ips := range domainsIP {
			go getLowRttIp(domain, ips)
		}

	}
	wg.Wait()

	//fmt.Println("解析到多个ip的待处理域名：\n", domainMultipleIPList)
	//fmt.Println("只解析到一个ip的域名：\n", domainIPList)
	//fmt.Printf("得到的所有ip: %v\n", domainIPList)

	//updateHostsFile()

	fmt.Printf("\nFetched %d/%d(total) sites in %.2fs seconds\n", len(domainIPList), len(GDomains), time.Since(start).Seconds())
}
