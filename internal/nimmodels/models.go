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

func GetV1ModelsURL(nimServiceEndpoint string) string {
	return nimServiceEndpoint + modelsV1URI
}
