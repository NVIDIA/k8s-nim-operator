package util

// Common values.
const (
	// Null values for these because they are needed.
	PVCStorageName  = ""
	PVCSize         = ""
	PVCStorageClass = ""

	// Default values.
	AuthSecret          = "ngc-api-secret"
	PVCCreate           = false
	PVCVolumeAccessMode = "ReadWriteMany"
	AltSecret			= "hf-api-secret"
)
var PullSecrets []string = []string{"ngc-secret"}

// NIMService-specific values.
const (
	ImageRepository              = ""
	Tag                          = ""
	NIMCacheStorageName          = ""
	NIMCacheStorageProfile       = ""
	ScaleMinReplicas       int32 = -1
	ScaleMaxReplicas       int32 = -1
	HostPath                     = ""

	PullPolicy              	= "IfNotPresent"
	ServicePort       int32 	= 8000
	ServiceType             	= "ClusterIP"
	GPULimit                	= "1"
	Replicas               		= 1
	InferencePlatform       	= "standalone"
)

// NIMCache-specific values.
const (
	SourceConfiguration          = ""
	ResourcesCPU                 = ""
	ResourcesMemory              = ""
	ModelPuller                  = ""
	ModelEndpoint                = ""
	Precision                    = ""
	Engine                       = ""
	TensorParallelism            = ""
	QosProfile                   = ""
	// Lora and Buildable are bools, but defined as a string for null value handling.
	Lora                         = ""
	Buildable                    = ""

	// Common endpoint & namespace for nemodatastore and hf. Specify what each does in tag description.
	AltEndpoint					 = ""
	AltNamespace				 = ""
	// DSHFCommonFields. AuthSecret, ModelPuller, PullSecret are reused.
	ModelName					 = ""
	DatasetName					 = ""
	Revision					 = ""

	// PullSecret for NIMCache, not PullSecrets
	PullSecret					 = "ngc-secret"
)
var Profiles []string = []string{}
var GPUs []string = []string{}
var Env []string = []string{}
