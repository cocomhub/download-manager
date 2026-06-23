// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tunnel

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type SclientConfig struct {
	ServerURL        string `yaml:"server_url"`
	UploadEndpoint   string `yaml:"upload_endpoint"`
	DownloadEndpoint string `yaml:"download_endpoint"`
	DeleteEndpoint   string `yaml:"delete_endpoint"`
	CheckMD5         bool   `yaml:"check_md5"`
	Timeout          int    `yaml:"timeout"`
	TunnelKey        string `yaml:"tunnel_key"`
	TunnelEndpoint   string `yaml:"tunnel_endpoint"`
}

func TunnelRequest(cfg *SclientConfig, method, targetURL string, headers map[string]string, body string, showHeaders, verbose bool) (string, error) {
	c, err := NewClient(cfg.TunnelKey, strings.TrimRight(cfg.ServerURL, "/")+cfg.TunnelEndpoint, time.Duration(cfg.Timeout)*time.Second)
	if err != nil {
		return "", fmt.Errorf("鍒涘缓 tunnel 瀹㈡埛绔け璐? %w", err)
	}
	req := &Request{
		Method:  method,
		URL:     targetURL,
		Headers: headers,
		Body:    EncodeBody([]byte(body)),
	}
	if verbose {
		payloadJSON, _ := json.Marshal(req)
		fmt.Fprintf(os.Stderr, "[璇锋眰杞借嵎] %s\n", string(payloadJSON))
		fmt.Fprintf(os.Stderr, "[Tunnel] POST %s => %s\n", c.TunnelURL, req.URL)
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("tunnel 璇锋眰澶辫触: %w", err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "[鍝嶅簲鐘舵€乚 %d\n", resp.Status)
	}
	if showHeaders {
		for k, v := range resp.Headers {
			fmt.Printf("%s: %s\n", k, v)
		}
		fmt.Println()
	}
	bodyBytes, err := DecodeBody(resp.Body)
	if err != nil {
		return "", fmt.Errorf("瑙ｇ爜鍝嶅簲浣撳け璐? %w", err)
	}
	return string(bodyBytes), nil
}

type Request struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type Response struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

func ParseKey(hexKey string) ([]byte, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes (64 hex chars)")
	}
	return key, nil
}

func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	return hex.EncodeToString(key), nil
}

func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func Decrypt(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

func EncodeBody(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func DecodeBody(encoded string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(encoded)
}

func NewHandler(key []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(key) == 0 {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		plaintext, err := Decrypt(key, body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var req Request
		if err := json.Unmarshal(plaintext, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var bodyReader io.Reader
		if req.Body != "" {
			decoded, err := DecodeBody(req.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			bodyReader = bytes.NewReader(decoded)
		}

		proxyReq, err := http.NewRequestWithContext(r.Context(), req.Method, req.URL, bodyReader)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		for k, v := range req.Headers {
			proxyReq.Header.Set(k, v)
		}

		client := &http.Client{}
		resp, err := client.Do(proxyReq)
		if err != nil {
			tunnelResp := Response{
				Status:  502,
				Headers: map[string]string{},
				Body:    EncodeBody([]byte(err.Error())),
			}
			writeEncryptedResponse(w, key, tunnelResp)
			return
		}
		defer resp.Body.Close()

		respBodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		respHeaders := make(map[string]string)
		for k := range resp.Header {
			respHeaders[k] = resp.Header.Get(k)
		}

		tunnelResp := Response{
			Status:  resp.StatusCode,
			Headers: respHeaders,
			Body:    EncodeBody(respBodyBytes),
		}
		writeEncryptedResponse(w, key, tunnelResp)
	})
}

func writeEncryptedResponse(w http.ResponseWriter, key []byte, resp Response) {
	respJSON, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	encrypted, err := Encrypt(key, respJSON)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encrypted)
}

type Client struct {
	Key        []byte
	TunnelURL  string
	HTTPClient *http.Client
}

func NewClient(hexKey, tunnelURL string, timeout time.Duration) (*Client, error) {
	key, err := ParseKey(hexKey)
	if err != nil {
		return nil, err
	}
	return &Client{
		Key:       key,
		TunnelURL: strings.TrimRight(tunnelURL, "/"),
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *Client) Do(req *Request) (*Response, error) {
	payloadJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	encrypted, err := Encrypt(c.Key, payloadJSON)
	if err != nil {
		return nil, fmt.Errorf("encrypt request: %w", err)
	}

	httpResp, err := c.HTTPClient.Post(c.TunnelURL, "application/octet-stream", bytes.NewReader(encrypted))
	if err != nil {
		return nil, fmt.Errorf("post request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tunnel error (HTTP %d): %s", httpResp.StatusCode, string(respBody))
	}

	decrypted, err := Decrypt(c.Key, respBody)
	if err != nil {
		return nil, fmt.Errorf("decrypt response: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(decrypted, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resp, nil
}
