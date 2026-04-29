package data_collector

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	AuthClientId      = "dummy-client-id"
	AuthClientSecret  = "dummy-client-secret"
	AuthScope         = "ihealth"
	AuthTenantId      = "aus10ke9yjejmNDY00h8"
	AuthGrantType     = "client_credentials"
	AuthTokenEndpoint = "https://identity.preprod.account.f5networks.net/oauth2/" + AuthTenantId + "/v1/token"
	UploadEndpoint    = "https://ihealth2-api.f5networks.net/qkview-analyzer/api/qkviews"
)

type authParams struct {
	ClientId     string
	ClientSecret string
	Scope        string
	TenantId     string
	GrantType    string
}

func Authenticate(dc *DataCollector) error {
	authParams := authParams{
		ClientId:     dc.IHealthCreds.ClientID,
		ClientSecret: dc.IHealthCreds.ClientSecret,
		Scope:        AuthScope,
		TenantId:     AuthTenantId,
		GrantType:    AuthGrantType,
	}

	// Authenticate and get token
	token, err := GetIdToken(authParams)
	if err != nil {
		return fmt.Errorf("authentication failed: %v", err)
	}

	dc.IHealthCreds.Token = token
	return nil
}

func GetIdToken(authParams authParams) (string, error) {
	// Prepare token request data
	data := url.Values{}
	data.Set("grant_type", authParams.GrantType)
	data.Set("scope", authParams.Scope)

	// Create a new request
	req, err := http.NewRequest("POST", AuthTokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(authParams.ClientId, authParams.ClientSecret)

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %v", err)
	}
	defer resp.Body.Close()

	// Read and parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	// See if there is an error; differentiate if it's a pending state or a permanent error
	if e, ok := result["error"].(string); ok {
		if e == "authorization_pending" {
			return "authorization_pending", nil
		}
		if desc, ok := result["error_description"].(string); ok {
			return "", fmt.Errorf("authorization error: %s - %s", e, desc)
		}
		return "", fmt.Errorf("authorization error: %v", e)
	}

	accessToken, ok := result["access_token"].(string)
	if !ok {
		// Fallback to id_token if access_token is not present, though typically client_credentials returns access_token
		if idToken, ok := result["id_token"].(string); ok {
			return idToken, nil
		}
		return "", fmt.Errorf("access_token not found in response: %s", string(body))
	}

	return accessToken, nil
}

type ihealthResponse struct {
	XMLName          xml.Name `xml:"qkview"`
	GuiURI           string   `xml:"gui_uri"`
	ProcessingStatus string   `xml:"processing_status"`
	FileName         string   `xml:"file_name"`
}

func UploadToIHealth(dc *DataCollector, packagePath string) error {
	file, err := os.Open(packagePath)
	if err != nil {
		return err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("qkview", filepath.Base(packagePath))
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return err
	}

	err = writer.WriteField("visible_in_gui", "true")
	if err != nil {
		return err
	}

	err = writer.Close()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", UploadEndpoint, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+dc.IHealthCreds.Token)
	req.Header.Set("Accept", "application/vnd.f5.ihealth.api")
	req.Header.Set("User-Agent", "nginx-supportpkg-for-k8s")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload to iHealth: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("bad status: %s, body: %s", resp.Status, string(respBody))
	}

	var ihealthResp ihealthResponse
	if err := xml.Unmarshal(respBody, &ihealthResp); err != nil {
		return fmt.Errorf("failed to parse XML response: %v", err)
	}

	fmt.Printf("Successfully uploaded %s to iHealth.\n", packagePath)
	if ihealthResp.FileName != "" {
		fmt.Printf("File Name: %s\n", ihealthResp.FileName)
	}
	if ihealthResp.ProcessingStatus != "" {
		fmt.Printf("Processing Status: %s\n", ihealthResp.ProcessingStatus)
	}
	if ihealthResp.GuiURI != "" {
		fmt.Printf("iHealth Link: %s\n", ihealthResp.GuiURI)
	}

	return nil
}
