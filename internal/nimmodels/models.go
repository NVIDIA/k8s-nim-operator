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
	modelsV1URI = "/v1/models"
)

type ModelsV1Info struct {
	Id     string     `json:"id"`
	Object ObjectType `json:"object"`
	Parent *string    `json:"parent,omitempty"`
	Root   *string    `json:"root,omitempty"`
}
type ModelsV1List struct {
	Object ObjectType     `json:"object"`
	Data   []ModelsV1Info `json:"data,omitempty"`
}

func getModelsV1URL(nimServiceEndpoint string) string {
	return fmt.Sprintf("http://%s%s", nimServiceEndpoint, modelsV1URI)
}

func ListModelsV1(ctx context.Context, nimServiceEndpoint string) (*ModelsV1List, error) {
	logger := log.FromContext(ctx)

	httpClient := http.Client{
		Timeout: 30 * time.Second,
	}
	modelsURL := getModelsV1URL(nimServiceEndpoint)
	modelsReq, err := http.NewRequest(http.MethodGet, modelsURL, nil)
	if err != nil {
		logger.Error(err, "failed to prepare request for models endpoint", "url", modelsURL)
		return nil, err
	}

	modelsResp, err := httpClient.Do(modelsReq)
	if err != nil {
		logger.Error(err, "failed to make request for models endpoint", "url", modelsURL)
		return nil, err
	}
	defer modelsResp.Body.Close()

	modelsData, err := io.ReadAll(modelsResp.Body)

	var modelsList ModelsV1List
	err = json.Unmarshal(modelsData, &modelsList)
	if err != nil {
		logger.Error(err, "failed to unmarshal models response", "url", modelsURL)
		return nil, err
	}

	return &modelsList, nil
}
