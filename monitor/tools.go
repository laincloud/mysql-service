package monitor

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ericpai/msops"
	"github.com/golang/glog"
)

func save(data, fileName string) error {
	file, err := os.Create(fileName)
	if err == nil {
		bfWt := bufio.NewWriter(file)
		defer file.Close()
		bfWt.WriteString(data)
		bfWt.Flush()
	}
	return err
}

func load(fileName string) ([]string, error) {
	var data []string
	file, err := os.Open(fileName)
	if err == nil {
		defer file.Close()
		bfRd := bufio.NewScanner(file)
		for bfRd.Scan() {
			data = append(data, bfRd.Text())
		}
		err = bfRd.Err()
	}
	return data, err
}

func GetLainConf(key string) ([]byte, error) {
	return lainletClient.Get("/v2/configwatcher?target="+key, 2*time.Second)
}

func getSecretConf() map[string]string {
	conf := make(map[string]string)
	if secretFile, err := os.Open(secretFileName); err == nil {
		defer secretFile.Close()
		bfRd := bufio.NewReader(secretFile)
		for {
			line, err := bfRd.ReadString('\n')
			line = strings.TrimRight(line, "\n")
			if err != nil || line == "" {
				break
			}
			if lineArr := strings.SplitN(line, "=", 2); len(lineArr) == 2 {
				conf[strings.TrimSpace(lineArr[0])] = strings.TrimSpace(lineArr[1])
			}
		}
	} else {
		glog.Errorf("Cann't open secret file: %s", secretFileName)
	}
	return conf
}

func getWebPrefix(appname string) string {
	segs := strings.Split(appname, ".")
	revSegs := make([]string, 0, len(segs))
	for i := len(segs) - 1; i >= 0; i-- {
		revSegs = append(revSegs, segs[i])
	}
	return strings.Join(revSegs, ".")
}

func prepareReportData(endpoint string) []string {
	data := make([]string, 0, 5)
	timestamp := time.Now().Unix()
	slaveSt, _ := msops.GetSlaveStatus(endpoint)
	threadsConenctedRes, _ := msops.GetGlobalStatus(endpoint, "Threads_Connected")
	questionsRes, _ := msops.GetGlobalStatus(endpoint, "Questions")
	var threadsConnected, questions int
	if value, exist := threadsConenctedRes["Threads_Connected"]; exist {
		threadsConnected, _ = strconv.Atoi(value)
	}
	if value, exist := questionsRes["Questions"]; exist {
		questions, _ = strconv.Atoi(value)
	}
	alive := 0
	if msops.CheckInstance(endpoint) == msops.InstanceOK {
		alive = 1
	}
	data = append(data,
		formatReportData(endpoint, "Slave_IO_Running", parseYesNo(slaveSt.SlaveIORunning), timestamp),
		formatReportData(endpoint, "Slave_SQL_Running", parseYesNo(slaveSt.SlaveSQLRunning), timestamp),
		formatReportData(endpoint, "Threads_connected", threadsConnected, timestamp),
		formatReportData(endpoint, "Questions", questions, timestamp),
		formatReportData(endpoint, "Alive", alive, timestamp),
	)
	return data
}

// formatReportData generates the reporting data for graphite.
// The format is: "domain.appname.proc_name-instance_no.key value timestamp"
func formatReportData(endpoint, key string, value int, timestamp int64) string {
	host, _, _ := net.SplitHostPort(endpoint)
	return fmt.Sprintf(reportFormat, graphiteKeyDomain, graphiteKeyAppName, host, key, value, timestamp)
}

func parseYesNo(value string) int {
	if value == "Yes" {
		return 1
	}
	return 0
}
