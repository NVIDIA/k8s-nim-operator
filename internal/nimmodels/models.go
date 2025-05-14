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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
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

func getURL(endpoint string, uri string) string {
	return fmt.Sprintf("http://%s%s", endpoint, uri)
}

func processAPIResponse(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	return newAPIError(resp)
}

func doGetRequest(ctx context.Context, url string) ([]byte, error) {
	logger := log.FromContext(ctx)

	httpClient := http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := httpClient.Get(url)
	if err != nil {
		logger.Error(err, "GET request failed", "url", url)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error(err, "Failed to read response", "url", url)
		return nil, err
	}
	logger.V(4).Info("DEBUG: API response", "endpoint", url, "body", string(body))

	err = processAPIResponse(resp)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func ListModelsV1(ctx context.Context, nimServiceEndpoint string) (*ModelsV1List, error) {
	logger := log.FromContext(ctx)

	modelsURL := getURL(nimServiceEndpoint, ModelsV1URI)
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

func GetMetadataV1(ctx context.Context, nimServiceEndpoint string) (*MetadataV1, error) {
	logger := log.FromContext(ctx)

	metadataURL := getURL(nimServiceEndpoint, MetadataV1URI)
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
