package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agilepanel/internal/config"
)

type ListBucketResult struct {
	XMLName  xml.Name `xml:"ListBucketResult"`
	Contents []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
}

// hashSHA256 returns the hex-encoded SHA256 hash of data.
func hashSHA256(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// signHMAC returns the HMAC-SHA256 signature of data with the given key.
func signHMAC(key []byte, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// getSignatureKey computes the Signature Key for AWS SigV4.
func getSignatureKey(secret, dateStamp, regionName, serviceName string) []byte {
	kDate := signHMAC([]byte("AWS4"+secret), []byte(dateStamp))
	kRegion := signHMAC(kDate, []byte(regionName))
	kService := signHMAC(kRegion, []byte(serviceName))
	kSigning := signHMAC(kService, []byte("aws4_request"))
	return kSigning
}

// getS3URL builds a standard S3 compatible endpoint URL.
func getS3URL(endpoint, bucket, s3Key string) string {
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimSuffix(endpoint, "/")

	isPathStyle := false
	if strings.Contains(endpoint, ":") || strings.Contains(endpoint, "127.0.0.1") || !strings.Contains(endpoint, ".") {
		isPathStyle = true
	}

	protocol := "https://"
	if strings.Contains(endpoint, "localhost") || strings.Contains(endpoint, "127.0.0.1") {
		protocol = "http://"
	}

	if isPathStyle {
		return fmt.Sprintf("%s%s/%s/%s", protocol, endpoint, bucket, strings.TrimPrefix(s3Key, "/"))
	}
	return fmt.Sprintf("%s%s.%s/%s", protocol, bucket, endpoint, strings.TrimPrefix(s3Key, "/"))
}

// buildS3Request builds and signs an HTTP request for S3 using SigV4.
func buildS3Request(method, urlStr, s3Key, region, bucket, accessKey, secretKey string, body io.ReadSeeker, bodyLen int64, contentType string) (*http.Request, error) {
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, err
	}

	t := time.Now().UTC()
	amzDate := t.Format("20060102T150405Z")
	dateStamp := t.Format("20060102")

	host := req.URL.Host
	req.Header.Set("Host", host)
	req.Header.Set("X-Amz-Date", amzDate)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	var payloadHash string
	if method == "PUT" && body != nil {
		h := sha256.New()
		_, _ = body.Seek(0, io.SeekStart)
		_, _ = io.Copy(h, body)
		payloadHash = hex.EncodeToString(h.Sum(nil))
		_, _ = body.Seek(0, io.SeekStart)
	} else {
		payloadHash = hashSHA256([]byte(""))
	}
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := req.URL.RawQuery

	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", host, payloadHash, amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s", method, canonicalURI, canonicalQuery, canonicalHeaders, signedHeaders, payloadHash)

	algorithm := "AWS4-HMAC-SHA256"
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	stringToSign := fmt.Sprintf("%s\n%s\n%s\n%s", algorithm, amzDate, credentialScope, hashSHA256([]byte(canonicalRequest)))

	signingKey := getSignatureKey(secretKey, dateStamp, region, "s3")
	signature := hex.EncodeToString(signHMAC(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s", algorithm, accessKey, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)

	if method == "PUT" {
		req.ContentLength = bodyLen
	}

	return req, nil
}

// UploadToS3 uploads a local file to S3 storage.
func UploadToS3(localPath string, s3Key string, state *config.State) error {
	endpoint := state.Global.S3Endpoint
	bucket := state.Global.S3Bucket
	region := state.Global.S3Region
	accessKey := state.Global.S3AccessKey
	secretKey := state.Global.S3SecretKey

	if endpoint == "" || bucket == "" || accessKey == "" || secretKey == "" {
		return fmt.Errorf("S3 credentials not set in global configurations")
	}

	if region == "" {
		region = "us-east-1"
	}

	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	urlStr := getS3URL(endpoint, bucket, s3Key)
	req, err := buildS3Request("PUT", urlStr, s3Key, region, bucket, accessKey, secretKey, file, stat.Size(), "application/zip")
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 15 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("S3 upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// ListS3Backups lists all object keys matching the prefix backups/[domain]/.
func ListS3Backups(domain string, state *config.State) ([]string, error) {
	endpoint := state.Global.S3Endpoint
	bucket := state.Global.S3Bucket
	region := state.Global.S3Region
	accessKey := state.Global.S3AccessKey
	secretKey := state.Global.S3SecretKey

	if endpoint == "" || bucket == "" || accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("S3 credentials not configured")
	}

	if region == "" {
		region = "us-east-1"
	}

	prefix := fmt.Sprintf("backups/%s/", domain)

	// Build the base URL and add the prefix as a properly encoded query parameter.
	// It must be part of the URL before signing so SigV4 covers the query string.
	baseURL := getS3URL(endpoint, bucket, "")
	params := url.Values{}
	params.Set("prefix", prefix)
	urlStr := baseURL + "?" + params.Encode()

	req, err := buildS3Request("GET", urlStr, "", region, bucket, accessKey, secretKey, nil, 0, "")
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("S3 list failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result ListBucketResult
	err = xml.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, fmt.Errorf("failed to decode S3 XML: %w", err)
	}

	var keys []string
	for _, content := range result.Contents {
		keys = append(keys, content.Key)
	}

	return keys, nil
}

// DownloadFromS3 downloads an object from S3 storage to local path.
func DownloadFromS3(s3Key string, localPath string, state *config.State) error {
	endpoint := state.Global.S3Endpoint
	bucket := state.Global.S3Bucket
	region := state.Global.S3Region
	accessKey := state.Global.S3AccessKey
	secretKey := state.Global.S3SecretKey

	if endpoint == "" || bucket == "" || accessKey == "" || secretKey == "" {
		return fmt.Errorf("S3 credentials not configured")
	}

	if region == "" {
		region = "us-east-1"
	}

	urlStr := getS3URL(endpoint, bucket, s3Key)
	req, err := buildS3Request("GET", urlStr, s3Key, region, bucket, accessKey, secretKey, nil, 0, "")
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 15 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("S3 download failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	_ = os.MkdirAll(filepath.Dir(localPath), 0755)
	outFile, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	return err
}

// DeleteFromS3 deletes an object from S3 storage.
func DeleteFromS3(s3Key string, state *config.State) error {
	endpoint := state.Global.S3Endpoint
	bucket := state.Global.S3Bucket
	region := state.Global.S3Region
	accessKey := state.Global.S3AccessKey
	secretKey := state.Global.S3SecretKey

	if accessKey == "" || secretKey == "" || bucket == "" {
		return fmt.Errorf("S3 credentials not configured")
	}

	if region == "" {
		region = "us-east-1"
	}

	urlStr := getS3URL(endpoint, bucket, s3Key)
	req, err := buildS3Request("DELETE", urlStr, s3Key, region, bucket, accessKey, secretKey, nil, 0, "")
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("S3 delete failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

