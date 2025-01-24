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

package v1alpha1

type ArgoWorkFlows struct {
	Endpoint       string `json:"endpoint"`
	ServiceAccount string `json:"serviceAccount"`
}

type Milvus struct {
	Endpoint string `json:"endpoint"`
}

type DataStore struct {
	Endpoint string `json:"endpoint"`
}

type DatabaseConfig struct {
	// Host is the hostname of the database.
	// Required, must not be empty.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host,omitempty"`
	// Port is the port where the database is reachable at.
	// If specified, this must be a valid port number, 0 < databasePort < 65536.
	// Defaults to 5432.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default:=5432
	Port int32 `json:"port,omitempty"`
	// DatabaseName is the database name for a NEMO Service.
	// Required, must not be empty.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	DatabaseName string `json:"databaseName,omitempty"`
	// DatabaseCredentials stores the configuration to retrieve the database credentials.
	// Required, must not be nil.
	//
	// +kubebuilder:validation:Required
	Credentials *DatabaseCredentials `json:"credentials,omitempty"`
}

type DatabaseCredentials struct {
	// User is the non-root username for a NEMO Service in the database.
	// Required, must not be empty.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	User string `json:"user,omitempty"`
	// SecretName is the name of the secret which has the database credentials for a NEMO service user.
	// Required, must not be empty.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SecretName string `json:"secretName,omitempty"`
	// PasswordKey is the name of the key in the `CredentialsSecret` secret for the database credentials.
	// Defaults to "password".
	//
	// +kubebuilder:default:="password"
	PasswordKey string `json:"passwordKey,omitempty"`
}
