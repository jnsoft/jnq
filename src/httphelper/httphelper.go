package httphelper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var sharedClient = &http.Client{}

var sharedClient2 = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 1000,
		IdleConnTimeout:     90 * time.Second,
	},
}

func GetJSON[T any](url string, headers ...[2]string) (T, int, error) {
	var result T
	resp, code, err := GetBytes(url, headers...)
	if err != nil || len(resp) == 0 {
		return result, code, err
	}

	err = json.Unmarshal(resp, &result)
	if err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		return result, 0, err
	}

	/* get as string:
	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, resp, "", "  ")
	if err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		return result, 0, err
	}
	*/
	return result, code, nil
}

func GetString(url string, headers ...[2]string) (string, int, error) {
	resp, code, err := GetBytes(url, headers...)
	if err != nil {
		return "", 0, err
	}

	if len(resp) == 0 {
		return "", code, nil
	}

	return string(resp), code, nil
}

func GetBytes(url string, headers ...[2]string) ([]byte, int, error) {
	resp, code, err := get(url, headers...)
	if err != nil {
		return nil, 0, err
	}

	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, code, readErr
	}
	return body, code, nil
}

func get(url string, headers ...[2]string) (*http.Response, int, error) {

	//sharedClient.Transport = &http.Transport{
	//	Proxy: nil,
	//	// ...other transport settings...
	//}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("Failed to create request for %s: %v\n", url, err)
		return nil, 0, err
	}

	for _, header := range headers {
		req.Header.Set(header[0], header[1])
	}

	resp, err := sharedClient.Do(req)

	if err != nil {
		return nil, 0, err
	}
	return resp, resp.StatusCode, nil
}

func PostJSON[T any](url string, data T, headers ...[2]string) (string, int, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", 0, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return PostBytes(url, jsonData, headers...)
}

func PostString(url string, data string, headers ...[2]string) (string, int, error) {
	return PostBytes(url, []byte(data), headers...)
}

func PostBytes(url string, data []byte, headers ...[2]string) (string, int, error) {
	resp, status, err := post(url, data, headers...)
	if err != nil {
		return "", 0, fmt.Errorf("failed to post data: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", status, fmt.Errorf("failed to read response body: %w", readErr)
	}

	return string(body), status, nil
}

func post(url string, data []byte, headers ...[2]string) (*http.Response, int, error) {
	req, err := http.NewRequest("POST", url, io.NopCloser(bytes.NewReader(data)))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	for _, header := range headers {
		req.Header.Set(header[0], header[1])
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := sharedClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to send request: %w", err)
	}

	return resp, resp.StatusCode, nil
}

func SplitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}
