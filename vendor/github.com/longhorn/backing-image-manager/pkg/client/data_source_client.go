package client

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/longhorn/backing-image-manager/api"
)

const (
	HTTPClientTimeout = 10 * time.Second
)

type DataSourceClient struct {
	Remote string
}

func (client *DataSourceClient) Get() (*api.DataSourceInfo, error) {
	httpClient := &http.Client{Timeout: HTTPClientTimeout}

	url := fmt.Sprintf("http://%s/v1/file", client.Remote)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get failed, err: %s", err)
	}
	defer resp.Body.Close()

	bodyContent, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		if err != nil {
			return nil, fmt.Errorf("resp.StatusCode(%d) != http.StatusOK, err is unknown", resp.StatusCode)
		}
		return nil, fmt.Errorf("resp.StatusCode(%d) != http.StatusOK, err: %v", resp.StatusCode, string(bodyContent))
	}

	result := &api.DataSourceInfo{}

	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(bodyContent, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (client *DataSourceClient) Upload(filePath string) error {
	httpClient := &http.Client{Timeout: 0}

	stat, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	r, w := io.Pipe()
	m := multipart.NewWriter(w)
	go func() {
		defer w.Close()
		defer m.Close()
		part, err := m.CreateFormFile("chunk", "blob")
		if err != nil {
			return
		}
		file, err := os.Open(filePath)
		if err != nil {
			return
		}
		defer file.Close()
		if _, err = io.Copy(part, file); err != nil {
			return
		}
	}()

	url := fmt.Sprintf("http://%s/v1/file", client.Remote)

	req, err := http.NewRequest("POST", url, r)
	if err != nil {
		return err
	}
	q := req.URL.Query()
	q.Add("action", "upload")
	q.Add("size", strconv.Itoa(int(stat.Size())))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Content-Type", m.FormDataContentType())

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload failed, err: %s", err)
	}
	defer resp.Body.Close()

	bodyContent, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		if err != nil {
			return fmt.Errorf("resp.StatusCode(%d) != http.StatusOK, err is unknown", resp.StatusCode)
		}
		return fmt.Errorf("resp.StatusCode(%d) != http.StatusOK, err: %v", resp.StatusCode, string(bodyContent))
	}

	return nil
}
