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

// DatabaseConfig is the external database configuration
type DatabaseConfig struct {
	// Host is the hostname of the database.
	// Required, must not be empty.
	//
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`
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
	// +kubebuilder:validation:MinLength=1
	DatabaseName string `json:"databaseName"`
	// DatabaseCredentials stores the configuration to retrieve the database credentials.
	// Required, must not be nil.
	//
	Credentials *DatabaseCredentials `json:"credentials"`
}

// DatabaseCredentials are the external database credentials
type DatabaseCredentials struct {
	// User is the non-root username for a NEMO Service in the database.
	// Required, must not be empty.
	//
	// +kubebuilder:validation:MinLength=1
	User string `json:"user"`
	// SecretName is the name of the secret which has the database credentials for a NEMO service user.
	// Required, must not be empty.
	//
	// +kubebuilder:validation:MinLength=1
	SecretName string `json:"secretName"`
	// PasswordKey is the name of the key in the `CredentialsSecret` secret for the database credentials.
	// Defaults to "password".
	//
	// +kubebuilder:default:="password"
	PasswordKey string `json:"passwordKey,omitempty"`
}

// WandBSecret represents the secret and key details for the Weights and Biases service.
type WandBSecret struct {
	// Name is the name of the Kubernetes Secret containing the WandB API key.
	// Required, must not be empty.
	//
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// APIKeyKey is the key in the Secret that holds the WandB API key.
	// Defaults to "apiKey".
	// +kubebuilder:default="apiKey"
	// +kubebuilder:validation:MinLength=1
	APIKeyKey string `json:"apiKeyKey"`

	// EncryptionKey is an optional key in the secret used for encrypting WandB credentials.
	// This can be used for additional security layers if required.
	// Defaults to "encryptionKey".
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="encryptionKey"
	EncryptionKey string `json:"encryptionKey,omitempty"`
}

// OTelSpec defines the settings for OpenTelemetry
type OTelSpec struct {
	// Enabled indicates if opentelemetry collector and tracing are enabled
	Enabled *bool `json:"enabled,omitempty"`

	// ExporterOtlpEndpoint is the OTLP collector endpoint.
	// +kubebuilder:validation:MinLength=1
	ExporterOtlpEndpoint string `json:"exporterOtlpEndpoint"`

	// DisableLogging indicates whether Python logging auto-instrumentation should be disabled.
	// +kubebuilder:validation:Optional
	DisableLogging *bool `json:"disableLogging,omitempty"`

	// ExporterConfig defines configuration for different OTel exporters
	// +kubebuilder:validation:Optional
	ExporterConfig ExporterConfig `json:"exporterConfig,omitempty"`

	// ExcludedUrls defines URLs to be excluded from tracing.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default={"health"}
	ExcludedUrls []string `json:"excludedUrls,omitempty"`

	// LogLevel defines the log level (e.g., INFO, DEBUG).
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="INFO"
	LogLevel string `json:"logLevel,omitempty"`
}

// ExporterConfig stores configuration for different OTel exporters
type ExporterConfig struct {
	// TracesExporter sets the traces exporter: (otlp, console, none).
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="otlp"
	TracesExporter string `json:"tracesExporter,omitempty"`

	// MetricsExporter sets the metrics exporter: (otlp, console, none).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=otlp;console;none
	// +kubebuilder:default="otlp"
	MetricsExporter string `json:"metricsExporter,omitempty"`

	// LogsExporter sets the logs exporter: (otlp, console, none).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=otlp;console;none
	// +kubebuilder:default="otlp"
	LogsExporter string `json:"logsExporter,omitempty"`
}
