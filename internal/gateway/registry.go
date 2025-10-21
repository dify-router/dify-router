package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// 在沙箱启动时调用
func RegisterWithGateway(sandboxID, sandboxType, gatewayURL string) error {
	instance := &SandboxInstance{
		ID:       sandboxID,
		URL:      fmt.Sprintf("http://%s:8194", sandboxID), // 使用配置的端口
		Type:     sandboxType,
		Status:   "healthy",
		Load:     0,
		LastPing: time.Now().Unix(),
	}

	client := &http.Client{Timeout: 10 * time.Second}
	instanceJSON, _ := json.Marshal(instance)

	req, err := http.NewRequest("POST", gatewayURL+"/admin/sandboxes/register", 
		bytes.NewBuffer(instanceJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("registration failed: %s", resp.Status)
	}

	return nil
}
