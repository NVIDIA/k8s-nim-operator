/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nimmodels

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// ServiceCABundlePath is the path where the OpenShift service-serving CA bundle
	// is mounted. OpenShift populates this via a ConfigMap annotated with
	// service.beta.openshift.io/inject-cabundle: "true".
	ServiceCABundlePath = "/etc/pki/tls/service-ca/service-ca.crt"

	// saTokenPath is the default path for the mounted ServiceAccount token.
	saTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

type ObjectType string

const (
	ObjectTypeList  ObjectType = "list"
	ObjectTypeModel ObjectType = "model"
)

const (
	ModelsV1URI   = "/v1/models"
	MetadataV1URI = "/v1/metadata"
)

type ModelsV1Info struct {
	Id     string     `json:"id"`
	Object ObjectType `json:"object"`
	Parent *string    `json:"parent,omitempty"`
	Root   *string    `json:"root,omitempty"`
}
type ModelsV1List struct {
	Object ObjectType     `json:"object"`
	Data   []ModelsV1Info `json:"data"`
}

type MetadataV1ModelInfo struct {
	ShortName string `json:"shortName"`
	ModelUrl  string `json:"modelUrl"`
}

type MetadataV1 struct {
	ModelInfo []MetadataV1ModelInfo `json:"modelInfo"`
}

func getURL(endpoint string, uri string, scheme string) string {
	if scheme != "" {
		return fmt.Sprintf("%s://%s%s", scheme, endpoint, uri)
	} else {
		return fmt.Sprintf("%s%s", endpoint, uri)
	}
}

func processAPIResponse(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	return newAPIError(resp)
}

// newTLSTransport creates an http.Transport configured with the CA bundle at the
// given path. Returns an error if the file cannot be read or parsed.
func newTLSTransport(caBundlePath string) (*http.Transport, error) {
	caCert, err := os.ReadFile(caBundlePath)
	if err != nil {
		return nil, fmt.Errorf("reading CA bundle from %s: %w", caBundlePath, err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("no valid certificates found in CA bundle at %s", caBundlePath)
	}

	return &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    caCertPool,
			MinVersion: tls.VersionTLS12,
		},
	}, nil
}

// readBearerToken reads a bearer token from the given file path.
// Returns the token string and true if successful, or empty string and false
// if the file doesn't exist or can't be read.
func readBearerToken(tokenPath string) (string, bool) {
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", false
	}
	t := strings.TrimSpace(string(token))
	if t == "" {
		return "", false
	}
	return t, true
}

func doGetRequest(ctx context.Context, requestURL string) ([]byte, error) {
	logger := log.FromContext(ctx)

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// For HTTPS URLs, configure TLS with the OpenShift service-serving CA bundle
	// and include the ServiceAccount bearer token for kube-rbac-proxy auth.
	parsedURL, err := url.Parse(requestURL)
	if err == nil && parsedURL.Scheme == "https" {
		transport, tlsErr := newTLSTransport(ServiceCABundlePath)
		if tlsErr != nil {
			logger.V(2).Info("Service CA bundle not available, using default TLS config", "path", ServiceCABundlePath, "error", tlsErr)
		} else {
			logger.V(2).Info("Using service CA bundle for HTTPS request", "path", ServiceCABundlePath)
			httpClient.Transport = transport
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		logger.Error(err, "Failed to create request", "url", requestURL)
		return nil, err
	}

	// When going through kube-rbac-proxy (HTTPS), include the ServiceAccount
	// bearer token for authentication.
	if parsedURL != nil && parsedURL.Scheme == "https" {
		if token, ok := readBearerToken(saTokenPath); ok {
			req.Header.Set("Authorization", "Bearer "+token)
		} else {
			logger.V(2).Info("ServiceAccount token not available, skipping auth")
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		logger.Error(err, "GET request failed", "url", requestURL)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error(err, "Failed to read response", "url", requestURL)
		return nil, err
	}
	logger.V(4).Info("DEBUG: API response", "endpoint", requestURL, "body", string(body))

	err = processAPIResponse(resp)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func ListModelsV1(ctx context.Context, nimServiceEndpoint string, scheme string) (*ModelsV1List, error) {
	logger := log.FromContext(ctx)

	modelsURL := getURL(nimServiceEndpoint, ModelsV1URI, scheme)
	modelsBytes, err := doGetRequest(ctx, modelsURL)
	if err != nil {
		return nil, err
	}

	var info ModelsV1List
	err = json.Unmarshal(modelsBytes, &info)
	if err != nil {
		logger.Error(err, "Failed to unmarshal models response", "url", modelsURL)
		return nil, err
	}

	return &info, nil
}

func GetMetadataV1(ctx context.Context, nimServiceEndpoint string, scheme string) (*MetadataV1, error) {
	logger := log.FromContext(ctx)

	metadataURL := getURL(nimServiceEndpoint, MetadataV1URI, scheme)
	metadataBytes, err := doGetRequest(ctx, metadataURL)
	if err != nil {
		return nil, err
	}

	var info MetadataV1
	err = json.Unmarshal(metadataBytes, &info)
	if err != nil {
		logger.Error(err, "Failed to unmarshal metadata response", "url", metadataURL)
		return nil, err
	}

	return &info, nil
}
