package download

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeFileMD5(t *testing.T) {
	// Create a temp file with known content
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	content := "hello"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	base64MD5, hexMD5, err := ComputeFileMD5(filePath)
	if err != nil {
		t.Fatalf("ComputeFileMD5 failed: %v", err)
	}

	// "hello" MD5 hex: 5d41402abc4b2a76b9719d911017c592
	expectedHex := "5d41402abc4b2a76b9719d911017c592"
	if hexMD5 != expectedHex {
		t.Errorf("expected hex MD5 %q, got %q", expectedHex, hexMD5)
	}

	// Verify base64 matches hex
	expectedBase64 := "XUFAKrxLKna5cZ2REBfFkg=="
	if base64MD5 != expectedBase64 {
		t.Errorf("expected base64 MD5 %q, got %q", expectedBase64, base64MD5)
	}
}

func TestComputeFileMD5EmptyFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(filePath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_, hexMD5, err := ComputeFileMD5(filePath)
	if err != nil {
		t.Fatalf("ComputeFileMD5 failed: %v", err)
	}

	// Empty file MD5
	if hexMD5 != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Errorf("unexpected hex MD5 for empty file: %s", hexMD5)
	}
}

func TestComputeFileMD5NotFound(t *testing.T) {
	_, _, err := ComputeFileMD5("/path/does/not/exist")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestTryGetMd5XAmzMetaMd5chksum(t *testing.T) {
	headers := map[string]string{
		"X-Amz-Meta-Md5chksum": "XUFAKrxLKna5cZ2REBfFkg==", // 24 chars base64
	}
	result := TryGetMd5(headers)
	if result != "XUFAKrxLKna5cZ2REBfFkg==" {
		t.Errorf("expected X-Amz-Meta-Md5chksum value, got %q", result)
	}
}

func TestTryGetMd5Etag(t *testing.T) {
	headers := map[string]string{
		"Etag": `"5d41402abc4b2a76b9719d911017c592"`, // 34 chars with quotes = 32 hex chars
	}
	result := TryGetMd5(headers)
	if result != "5d41402abc4b2a76b9719d911017c592" {
		t.Errorf("expected Etag hex value, got %q", result)
	}
}

func TestTryGetMd5ContentMD5(t *testing.T) {
	headers := map[string]string{
		"Content-MD5": "5d41402abc4b2a76b9719d911017c592", // 32 hex chars
	}
	result := TryGetMd5(headers)
	if result != "5d41402abc4b2a76b9719d911017c592" {
		t.Errorf("expected Content-MD5 value, got %q", result)
	}
}

func TestTryGetMd5Empty(t *testing.T) {
	result := TryGetMd5(map[string]string{})
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestTryGetMd5Nil(t *testing.T) {
	result := TryGetMd5(nil)
	if result != "" {
		t.Errorf("expected empty string for nil headers, got %q", result)
	}
}

func TestTryGetMd5EtagWeak(t *testing.T) {
	// The original implementation in dlcore/client.go handles weak ETags (W/"...")
	// but our new implementation follows the task spec which only checks:
	// len == 34 && [0]=='"' && [33]=='"'
	// A weak ETag like W/"xxx" is 36 chars, not 34, so it won't match.
	headers := map[string]string{
		"Etag": `"5d41402abc4b2a76b9719d911017c592"`,
	}
	result := TryGetMd5(headers)
	expected := "5d41402abc4b2a76b9719d911017c592"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestTryGetMd5InvalidLengthEtag(t *testing.T) {
	// Too short
	headers := map[string]string{
		"Etag": `"abc"`,
	}
	result := TryGetMd5(headers)
	if result != "" {
		t.Errorf("expected empty for short etag, got %q", result)
	}
}