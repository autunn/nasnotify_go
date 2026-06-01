package nas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nasnotify-go/internal/crypto"
)

type UGreenNotice struct {
	Time int64  `json:"time"`
	Body string `json:"body"`
}

type UGreenListResp struct {
	Code int `json:"code"`
	Data struct {
		List []UGreenNotice `json:"List"`
	} `json:"data"`
}

type UGreenSystemInfo struct {
	UsageCpu    float64 `json:"usageCpu"`
	CpuTemp     float64 `json:"cpuTemp"`
	CpuFan      int     `json:"cpuFan"`
	DeviceFan   int     `json:"deviceFan"`
	UsageMemory float64 `json:"usageMemory"`
	MemoryUsed  int64   `json:"memoryUsed"`
	MemoryTotal int64   `json:"memoryTotal"`

	NetworkReceive       string  `json:"networkReceive"`
	NetworkTransmit      string  `json:"networkTransmit"`
	NetworkReceiveValue  float64 `json:"networkReceiveValue"`
	NetworkReceiveUnit   string  `json:"networkReceiveUnit"`
	NetworkTransmitValue float64 `json:"networkTransmitValue"`
	NetworkTransmitUnit  string  `json:"networkTransmitUnit"`

	System  UGreenSystemStatus  `json:"system"`
	Storage []UGreenStorageItem `json:"storage"`
}

type UGreenSystemStatus struct {
	DevName       string              `json:"dev_name"`
	SystemVersion string              `json:"system_version"`
	Message       string              `json:"message"`
	TotalRunTime  int                 `json:"total_run_time"`
	ServerStatus  int                 `json:"server_status"`
	Status        int                 `json:"status"`
	LastBootDate  string              `json:"last_boot_date"`
	LastBootTime  int64               `json:"last_boot_time"`
	NetworkInfo   []UGreenNetworkInfo `json:"network_info"`
}

type UGreenNetworkInfo struct {
	IPv4  string `json:"ipv4"`
	IPv6  string `json:"ipv6"`
	Label string `json:"label"`
}

type UGreenStorageItem struct {
	Name        string `json:"name"`
	PoolName    string `json:"pool_name"`
	Size        int64  `json:"size"`
	Used        int64  `json:"used"`
	Status      int    `json:"status"`
	Description string `json:"description"`
	StorageName string `json:"storage_name"`
	NotifyPct   int    `json:"capacity_notify_percentage"`
}

func fetchUGreenNotices(authInfo *UGreenAuthInfo, ip string, port int, useSSL bool) ([]UGreenNotice, int, error) {
	protocol := "http"
	if useSSL {
		protocol = "https"
	}
	urlStr := fmt.Sprintf("%s://%s:%d/ugreen/v1/desktop/message/list", protocol, ip, port)

	payload := map[string]interface{}{"level": []string{"info", "important", "warning"}, "page": 1, "size": 10}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequest("POST", urlStr, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, 0, err
	}

	encToken, err := crypto.RsaEncrypt(authInfo.PublicKey, authInfo.Token)
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("x-specify-language", "zh-CN")
	req.Header.Set("x-ugreen-security-key", authInfo.TokenID)
	req.Header.Set("x-ugreen-token", encToken)
	if authInfo.CookieStr != "" {
		req.Header.Set("Cookie", authInfo.CookieStr)
	}

	client := newUGreenHTTPClient(10*time.Second, nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, 0, fmt.Errorf("notice http status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var listResp UGreenListResp
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, 0, err
	}

	return listResp.Data.List, listResp.Code, nil
}

func fetchUGreenSystemInfo(authInfo *UGreenAuthInfo, ip string, port int, useSSL bool) (*UGreenSystemInfo, error) {
	protocol := "http"
	if useSSL {
		protocol = "https"
	}
	baseURL := fmt.Sprintf("%s://%s:%d", protocol, ip, port)
	client := newUGreenHTTPClient(10*time.Second, nil)
	info := &UGreenSystemInfo{}

	doGet := func(apiPath string) ([]byte, error) {
		req, err := http.NewRequest("GET", baseURL+apiPath, nil)
		if err != nil {
			return nil, err
		}
		encToken, err := crypto.RsaEncrypt(authInfo.PublicKey, authInfo.Token)
		if err != nil {
			return nil, err
		}

		req.Header.Set("x-specify-language", "zh-CN")
		req.Header.Set("x-ugreen-security-key", authInfo.TokenID)
		req.Header.Set("x-ugreen-token", encToken)
		if authInfo.CookieStr != "" {
			req.Header.Set("Cookie", authInfo.CookieStr)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= http.StatusBadRequest {
			return nil, fmt.Errorf("api http status %d", resp.StatusCode)
		}

		var apiResp struct {
			Code int             `json:"code"`
			Msg  string          `json:"msg"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &apiResp); err == nil && (apiResp.Code != 0 || len(apiResp.Data) > 0 || apiResp.Msg != "") {
			if apiResp.Code != 0 && apiResp.Code != 200 {
				return nil, fmt.Errorf("api error: %d %s", apiResp.Code, apiResp.Msg)
			}
			if len(apiResp.Data) > 0 {
				return apiResp.Data, nil
			}
		}
		return raw, nil
	}

	widgetsRaw, err := doGet("/ugreen/v1/desktop/components")
	if err != nil {
		widgetsRaw, err = doGet("/ugreen/v2/desktop/components")
		if err != nil {
			return nil, err
		}
	}

	var widgets []struct {
		ID   string  `json:"id"`
		Type float64 `json:"type"`
	}
	if json.Unmarshal(widgetsRaw, &widgets) != nil || len(widgets) == 0 {
		var wrapped struct {
			Result []struct {
				ID   string  `json:"id"`
				Type float64 `json:"type"`
			} `json:"result"`
		}
		json.Unmarshal(widgetsRaw, &wrapped)
		widgets = wrapped.Result
	}
	if len(widgets) == 0 {
		return nil, fmt.Errorf("desktop components response is empty or invalid")
	}

	for _, w := range widgets {
		dataRaw, err := doGet(fmt.Sprintf("/ugreen/v1/desktop/components/data?id=%s", w.ID))
		if err != nil {
			continue
		}

		var wrapper map[string]json.RawMessage
		if json.Unmarshal(dataRaw, &wrapper) == nil {
			if data, ok := wrapper["data"]; ok && len(data) > 0 {
				dataRaw = data
			} else if result, ok := wrapper["result"]; ok && len(result) > 0 {
				dataRaw = result
			}
		}

		var raw map[string]interface{}
		if json.Unmarshal(dataRaw, &raw) != nil {
			continue
		}

		wType, _ := raw["type"].(float64)
		if int(wType) == 2 {
			json.Unmarshal(dataRaw, &info.System)
		} else if int(wType) == 4 {
			var list struct {
				StorageList []UGreenStorageItem `json:"storage_list"`
			}
			json.Unmarshal(dataRaw, &list)
			info.Storage = list.StorageList
		}
	}

	statRaw, err := doGet("/ugreen/v1/taskmgr/stat/get_all")
	if err == nil {
		parseUGreenTaskmgrStats(statRaw, info)
	}

	return info, nil
}

func parseUGreenTaskmgrStats(raw []byte, info *UGreenSystemInfo) {
	type statPoint struct {
		UsedPercent    float64 `json:"used_percent"`
		Temp           float64 `json:"temp"`
		Temperature    float64 `json:"temperature"`
		CPUTemp        float64 `json:"cpu_temp"`
		CPUTemperature float64 `json:"cpu_temperature"`
		RecvRate       float64 `json:"recv_rate"`
		SendRate       float64 `json:"send_rate"`
		Speed          int     `json:"speed"`
	}
	type statData struct {
		Overview struct {
			CPU       []statPoint `json:"cpu"`
			Mem       []statPoint `json:"mem"`
			Net       []statPoint `json:"net"`
			CpuFan    []statPoint `json:"cpu_fan"`
			DeviceFan []statPoint `json:"device_fan"`
		} `json:"overview"`
		CPU struct {
			Series []statPoint `json:"series"`
		} `json:"cpu"`
		Mem struct {
			Series    []statPoint `json:"series"`
			Structure struct {
				Used  int64 `json:"used"`
				Total int64 `json:"total"`
			} `json:"structure"`
		} `json:"mem"`
		Net struct {
			Series []statPoint `json:"series"`
		} `json:"net"`
	}
	hasData := func(data statData) bool {
		return len(data.Overview.CPU) > 0 ||
			len(data.Overview.Mem) > 0 ||
			len(data.Overview.Net) > 0 ||
			len(data.Overview.CpuFan) > 0 ||
			len(data.Overview.DeviceFan) > 0 ||
			len(data.CPU.Series) > 0 ||
			len(data.Mem.Series) > 0 ||
			len(data.Net.Series) > 0 ||
			data.Mem.Structure.Used > 0 ||
			data.Mem.Structure.Total > 0
	}

	var wrapped struct {
		Data statData `json:"data"`
	}
	var data statData
	if json.Unmarshal(raw, &wrapped) == nil && hasData(wrapped.Data) {
		data = wrapped.Data
	} else if json.Unmarshal(raw, &data) != nil {
		return
	}

	if len(data.Overview.CPU) > 0 {
		info.UsageCpu = data.Overview.CPU[0].UsedPercent
		info.CpuTemp = statPointTemperature(data.Overview.CPU[0])
	} else if len(data.CPU.Series) > 0 {
		info.UsageCpu = data.CPU.Series[0].UsedPercent
		info.CpuTemp = statPointTemperature(data.CPU.Series[0])
	}
	if len(data.Overview.CpuFan) > 0 {
		info.CpuFan = data.Overview.CpuFan[0].Speed
	}
	if len(data.Overview.DeviceFan) > 0 {
		info.DeviceFan = data.Overview.DeviceFan[0].Speed
	}
	if len(data.Overview.Mem) > 0 {
		info.UsageMemory = data.Overview.Mem[0].UsedPercent
	} else if len(data.Mem.Series) > 0 {
		info.UsageMemory = data.Mem.Series[0].UsedPercent
	}

	info.MemoryUsed = data.Mem.Structure.Used
	info.MemoryTotal = data.Mem.Structure.Total

	var recvRate, sendRate float64
	if len(data.Overview.Net) > 0 {
		recvRate = data.Overview.Net[0].RecvRate
		sendRate = data.Overview.Net[0].SendRate
	} else if len(data.Net.Series) > 0 {
		recvRate = data.Net.Series[0].RecvRate
		sendRate = data.Net.Series[0].SendRate
	}

	info.NetworkReceiveValue, info.NetworkTransmitValue = recvRate, sendRate
	info.NetworkReceive, _ = formatUGreenSpeed(recvRate)
	info.NetworkTransmit, _ = formatUGreenSpeed(sendRate)
}

func statPointTemperature(point struct {
	UsedPercent    float64 `json:"used_percent"`
	Temp           float64 `json:"temp"`
	Temperature    float64 `json:"temperature"`
	CPUTemp        float64 `json:"cpu_temp"`
	CPUTemperature float64 `json:"cpu_temperature"`
	RecvRate       float64 `json:"recv_rate"`
	SendRate       float64 `json:"send_rate"`
	Speed          int     `json:"speed"`
}) float64 {
	for _, value := range []float64{point.Temp, point.Temperature, point.CPUTemp, point.CPUTemperature} {
		if value > 0 {
			return value
		}
	}
	return 0
}
