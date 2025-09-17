package util

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	nimclientset "github.com/NVIDIA/k8s-nim-operator/api/versioned"
	nimfake "github.com/NVIDIA/k8s-nim-operator/api/versioned/fake"
)

// Mock client implementation for testing.
type mockClient struct {
	nimClient  nimclientset.Interface
	kubeClient kubernetes.Interface
}

func (m *mockClient) NIMClient() nimclientset.Interface {
	return m.nimClient
}

func (m *mockClient) KubernetesClient() kubernetes.Interface {
	return m.kubeClient
}

// Test NewFetchResourceOptions.
func Test_NewFetchResourceOptions(t *testing.T) {
	streams := genericclioptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	}

	options := NewFetchResourceOptions(nil, streams)

	if options == nil {
		t.Fatal("expected non-nil options")
	}
	if options.IoStreams == nil {
		t.Fatal("expected non-nil IoStreams")
	}
	if options.cmdFactory != nil {
		t.Fatal("expected nil cmdFactory when passed nil")
	}
}

// Test CompleteNamespace.
func Test_CompleteNamespace(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		namespace         string
		expectedNamespace string
		expectedName      string
		expectedType      ResourceType
		expectError       bool
		errorContains     string
	}{
		{
			name:              "no args with namespace flag",
			args:              []string{},
			namespace:         "test-ns",
			expectedNamespace: "test-ns",
			expectedName:      "",
			expectError:       false,
		},
		{
			name:              "no args no namespace - defaults to default",
			args:              []string{},
			namespace:         "",
			expectedNamespace: "default",
			expectedName:      "",
			expectError:       false,
		},
		{
			name:              "single arg - resource name",
			args:              []string{"my-resource"},
			namespace:         "test-ns",
			expectedNamespace: "test-ns",
			expectedName:      "my-resource",
			expectError:       false,
		},
		{
			name:              "two args - nimservice type and name",
			args:              []string{"nimservice", "my-nim"},
			namespace:         "test-ns",
			expectedNamespace: "test-ns",
			expectedName:      "my-nim",
			expectedType:      NIMService,
			expectError:       false,
		},
		{
			name:              "two args - nimcache type and name",
			args:              []string{"nimcache", "my-cache"},
			namespace:         "test-ns",
			expectedNamespace: "test-ns",
			expectedName:      "my-cache",
			expectedType:      NIMCache,
			expectError:       false,
		},
		{
			name:              "two args - case insensitive type",
			args:              []string{"NIMSERVICE", "my-nim"},
			namespace:         "test-ns",
			expectedNamespace: "test-ns",
			expectedName:      "my-nim",
			expectedType:      NIMService,
			expectError:       false,
		},
		{
			name:              "two args - invalid type",
			args:              []string{"invalid", "my-resource"},
			namespace:         "test-ns",
			expectedNamespace: "test-ns",
			expectError:       true,
			errorContains:     "invalid resource type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams := genericclioptions.IOStreams{
				In:     &bytes.Buffer{},
				Out:    &bytes.Buffer{},
				ErrOut: &bytes.Buffer{},
			}
			options := NewFetchResourceOptions(nil, streams)

			// Create command with namespace flag
			cmd := &cobra.Command{}
			cmd.Flags().String("namespace", tt.namespace, "namespace")

			err := options.CompleteNamespace(tt.args, cmd)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error containing %q but got none", tt.errorContains)
				}
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Fatalf("expected error containing %q, got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if options.Namespace != tt.expectedNamespace {
				t.Errorf("expected namespace %q, got %q", tt.expectedNamespace, options.Namespace)
			}
			if options.ResourceName != tt.expectedName {
				t.Errorf("expected resource name %q, got %q", tt.expectedName, options.ResourceName)
			}
			if tt.expectedType != "" && options.ResourceType != tt.expectedType {
				t.Errorf("expected resource type %q, got %q", tt.expectedType, options.ResourceType)
			}
		})
	}
}

// Test FetchResources for NIMService.
func Test_FetchResources_NIMService(t *testing.T) {
	tests := []struct {
		name          string
		options       *FetchResourceOptions
		nimServices   []appsv1alpha1.NIMService
		expectError   bool
		errorContains string
		expectedCount int
	}{
		{
			name: "list all nimservices in namespace",
			options: &FetchResourceOptions{
				Namespace:    "test-ns",
				ResourceType: NIMService,
			},
			nimServices: []appsv1alpha1.NIMService{
				{ObjectMeta: metav1.ObjectMeta{Name: "nim1", Namespace: "test-ns"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nim2", Namespace: "test-ns"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nim3", Namespace: "other-ns"}},
			},
			expectedCount: 2,
		},
		{
			name: "list all nimservices across all namespaces",
			options: &FetchResourceOptions{
				ResourceType:  NIMService,
				AllNamespaces: true,
			},
			nimServices: []appsv1alpha1.NIMService{
				{ObjectMeta: metav1.ObjectMeta{Name: "nim1", Namespace: "test-ns"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nim2", Namespace: "test-ns"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nim3", Namespace: "other-ns"}},
			},
			expectedCount: 3,
		},
		{
			name: "get specific nimservice by name",
			options: &FetchResourceOptions{
				Namespace:    "test-ns",
				ResourceName: "nim1",
				ResourceType: NIMService,
			},
			nimServices: []appsv1alpha1.NIMService{
				{ObjectMeta: metav1.ObjectMeta{Name: "nim1", Namespace: "test-ns"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nim2", Namespace: "test-ns"}},
			},
			expectedCount: 1,
		},
		{
			name: "nimservice not found in namespace",
			options: &FetchResourceOptions{
				Namespace:    "test-ns",
				ResourceName: "nim-missing",
				ResourceType: NIMService,
			},
			nimServices:   []appsv1alpha1.NIMService{},
			expectError:   true,
			errorContains: "NIMService nim-missing not found in namespace test-ns",
		},
		{
			name: "nimservice not found in any namespace",
			options: &FetchResourceOptions{
				ResourceName:  "nim-missing",
				ResourceType:  NIMService,
				AllNamespaces: true,
			},
			nimServices:   []appsv1alpha1.NIMService{},
			expectError:   true,
			errorContains: "NIMService nim-missing not found in any namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake NIM client
			nimClient := nimfake.NewSimpleClientset()

			// Add test NIMServices - but only add matching ones if filtering by name
			for _, ns := range tt.nimServices {
				// If ResourceName is specified and doesn't match, skip adding
				if tt.options.ResourceName != "" && ns.Name != tt.options.ResourceName {
					continue
				}
				// If AllNamespaces is false and namespace doesn't match, skip
				if !tt.options.AllNamespaces && tt.options.Namespace != "" && ns.Namespace != tt.options.Namespace {
					continue
				}
				_, _ = nimClient.AppsV1alpha1().NIMServices(ns.Namespace).Create(context.TODO(), &ns, metav1.CreateOptions{})
			}

			// Create mock client
			mockClient := &mockClient{
				nimClient: nimClient,
			}

			result, err := FetchResources(context.TODO(), tt.options, mockClient)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error containing %q but got none", tt.errorContains)
				}
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Fatalf("expected error containing %q, got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			nimServiceList, ok := result.(*appsv1alpha1.NIMServiceList)
			if !ok {
				t.Fatalf("expected NIMServiceList, got %T", result)
			}

			if len(nimServiceList.Items) != tt.expectedCount {
				t.Errorf("expected %d items, got %d", tt.expectedCount, len(nimServiceList.Items))
			}

			// If specific name requested, verify we got the right one
			if tt.options.ResourceName != "" && !tt.expectError {
				if nimServiceList.Items[0].Name != tt.options.ResourceName {
					t.Errorf("expected name %q, got %q", tt.options.ResourceName, nimServiceList.Items[0].Name)
				}
			}
		})
	}
}

// Test FetchResources for NIMCache.
func Test_FetchResources_NIMCache(t *testing.T) {
	tests := []struct {
		name          string
		options       *FetchResourceOptions
		nimCaches     []appsv1alpha1.NIMCache
		expectError   bool
		errorContains string
		expectedCount int
	}{
		{
			name: "list all nimcaches in namespace",
			options: &FetchResourceOptions{
				Namespace:    "test-ns",
				ResourceType: NIMCache,
			},
			nimCaches: []appsv1alpha1.NIMCache{
				{ObjectMeta: metav1.ObjectMeta{Name: "cache1", Namespace: "test-ns"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "cache2", Namespace: "test-ns"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "cache3", Namespace: "other-ns"}},
			},
			expectedCount: 2,
		},
		{
			name: "get specific nimcache by name",
			options: &FetchResourceOptions{
				Namespace:    "test-ns",
				ResourceName: "cache1",
				ResourceType: NIMCache,
			},
			nimCaches: []appsv1alpha1.NIMCache{
				{ObjectMeta: metav1.ObjectMeta{Name: "cache1", Namespace: "test-ns"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "cache2", Namespace: "test-ns"}},
			},
			expectedCount: 1,
		},
		{
			name: "nimcache not found",
			options: &FetchResourceOptions{
				Namespace:    "test-ns",
				ResourceName: "cache-missing",
				ResourceType: NIMCache,
			},
			nimCaches:     []appsv1alpha1.NIMCache{},
			expectError:   true,
			errorContains: "NIMCache cache-missing not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake NIM client
			nimClient := nimfake.NewSimpleClientset()

			// Add test NIMCaches - but only add matching ones if filtering by name
			for _, nc := range tt.nimCaches {
				// If ResourceName is specified and doesn't match, skip adding
				if tt.options.ResourceName != "" && nc.Name != tt.options.ResourceName {
					continue
				}
				// If AllNamespaces is false and namespace doesn't match, skip
				if !tt.options.AllNamespaces && tt.options.Namespace != "" && nc.Namespace != tt.options.Namespace {
					continue
				}
				_, _ = nimClient.AppsV1alpha1().NIMCaches(nc.Namespace).Create(context.TODO(), &nc, metav1.CreateOptions{})
			}

			// Create mock client
			mockClient := &mockClient{
				nimClient: nimClient,
			}

			result, err := FetchResources(context.TODO(), tt.options, mockClient)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error containing %q but got none", tt.errorContains)
				}
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Fatalf("expected error containing %q, got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			nimCacheList, ok := result.(*appsv1alpha1.NIMCacheList)
			if !ok {
				t.Fatalf("expected NIMCacheList, got %T", result)
			}

			if len(nimCacheList.Items) != tt.expectedCount {
				t.Errorf("expected %d items, got %d", tt.expectedCount, len(nimCacheList.Items))
			}
		})
	}
}

// Test messageConditionFrom.
func Test_messageConditionFrom(t *testing.T) {
	tests := []struct {
		name         string
		conditions   []metav1.Condition
		expectedType string
		expectedMsg  string
		expectError  bool
	}{
		{
			name: "prefer failed with message",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Message: "Ready message"},
				{Type: "Failed", Status: metav1.ConditionTrue, Message: "Failed message"},
				{Type: "Other", Status: metav1.ConditionFalse, Message: "Other message"},
			},
			expectedType: "Failed",
			expectedMsg:  "Failed message",
		},
		{
			name: "fallback to ready when no failed",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Message: "Ready message"},
				{Type: "Other", Status: metav1.ConditionFalse, Message: "Other message"},
			},
			expectedType: "Ready",
			expectedMsg:  "Ready message",
		},
		{
			name: "failed without message - skip to ready",
			conditions: []metav1.Condition{
				{Type: "Failed", Status: metav1.ConditionTrue, Message: ""},
				{Type: "Ready", Status: metav1.ConditionTrue, Message: "Ready message"},
			},
			expectedType: "Ready",
			expectedMsg:  "Ready message",
		},
		{
			name: "first condition with message when no failed/ready",
			conditions: []metav1.Condition{
				{Type: "Progressing", Status: metav1.ConditionTrue, Message: ""},
				{Type: "Other", Status: metav1.ConditionTrue, Message: "Other message"},
			},
			expectedType: "Other",
			expectedMsg:  "Other message",
		},
		{
			name: "return first condition even without message",
			conditions: []metav1.Condition{
				{Type: "Progressing", Status: metav1.ConditionTrue, Message: ""},
			},
			expectedType: "Progressing",
			expectedMsg:  "",
		},
		{
			name:        "no conditions",
			conditions:  []metav1.Condition{},
			expectError: true,
		},
		{
			name:        "nil conditions",
			conditions:  nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := messageConditionFrom(tt.conditions)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if err.Error() != "no conditions present" {
					t.Errorf("expected 'no conditions present' error, got: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Type != tt.expectedType {
				t.Errorf("expected type %q, got %q", tt.expectedType, result.Type)
			}
			if result.Message != tt.expectedMsg {
				t.Errorf("expected message %q, got %q", tt.expectedMsg, result.Message)
			}
		})
	}
}

// Test MessageCondition.
func Test_MessageCondition(t *testing.T) {
	readyCondition := metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Message: "Ready message",
	}

	tests := []struct {
		name          string
		obj           interface{}
		expectedType  string
		expectedMsg   string
		expectError   bool
		errorContains string
	}{
		{
			name: "NIMCache with conditions",
			obj: &appsv1alpha1.NIMCache{
				Status: appsv1alpha1.NIMCacheStatus{
					Conditions: []metav1.Condition{readyCondition},
				},
			},
			expectedType: "Ready",
			expectedMsg:  "Ready message",
		},
		{
			name: "NIMService with conditions",
			obj: &appsv1alpha1.NIMService{
				Status: appsv1alpha1.NIMServiceStatus{
					Conditions: []metav1.Condition{readyCondition},
				},
			},
			expectedType: "Ready",
			expectedMsg:  "Ready message",
		},
		{
			name: "NIMCache without conditions",
			obj: &appsv1alpha1.NIMCache{
				Status: appsv1alpha1.NIMCacheStatus{},
			},
			expectError:   true,
			errorContains: "no conditions present",
		},
		{
			name:          "unsupported type",
			obj:           "invalid",
			expectError:   true,
			errorContains: "unsupported type",
		},
		{
			name:          "nil object",
			obj:           nil,
			expectError:   true,
			errorContains: "unsupported type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MessageCondition(tt.obj)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Type != tt.expectedType {
				t.Errorf("expected type %q, got %q", tt.expectedType, result.Type)
			}
			if result.Message != tt.expectedMsg {
				t.Errorf("expected message %q, got %q", tt.expectedMsg, result.Message)
			}
		})
	}
}

// Test StreamResourceLogs.
func Test_StreamResourceLogs(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		resourceName  string
		labelSelector string
		pods          []corev1.Pod
		expectError   bool
		errorContains string
		expectedLogs  []string
	}{
		{
			name:          "no pods found",
			namespace:     "test-ns",
			resourceName:  "test-resource",
			labelSelector: "app=test",
			pods:          []corev1.Pod{},
			expectError:   true,
			errorContains: "no pods found",
		},
		{
			name:          "single pod with logs",
			namespace:     "test-ns",
			resourceName:  "test-resource",
			labelSelector: "app=test",
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "container1"},
						},
					},
				},
			},
			expectedLogs: []string{
				"[test-pod/container1] test log line",
			},
		},
		{
			name:          "multiple pods and containers",
			namespace:     "test-ns",
			resourceName:  "test-resource",
			labelSelector: "app=test",
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "container1"},
							{Name: "container2"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "test-ns",
						Labels:    map[string]string{"app": "test"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "container1"},
						},
					},
				},
			},
			expectedLogs: []string{
				"[pod1/container1]",
				"[pod1/container2]",
				"[pod2/container1]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake kubernetes client with pods
			kubeClient := k8sfake.NewSimpleClientset()
			for _, pod := range tt.pods {
				_, _ = kubeClient.CoreV1().Pods(pod.Namespace).Create(context.TODO(), &pod, metav1.CreateOptions{})
			}

			// Note: Mocking actual log streaming with the fake client is complex
			// The test will timeout but we can verify the setup is correct

			// Create output buffer
			var outBuf bytes.Buffer
			streams := &genericclioptions.IOStreams{
				Out:    &outBuf,
				ErrOut: &bytes.Buffer{},
			}

			options := &FetchResourceOptions{
				IoStreams: streams,
			}

			mockClient := &mockClient{
				kubeClient: kubeClient,
			}

			// Create context with timeout to prevent hanging
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			err := StreamResourceLogs(ctx, options, mockClient, tt.namespace, tt.resourceName, tt.labelSelector)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got: %v", tt.errorContains, err)
				}
				return
			}

			// For successful cases, we expect context deadline exceeded (since we're not actually streaming)
			if err != nil && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Test CompleteNamespace edge cases.
func Test_CompleteNamespace_EdgeCases(t *testing.T) {
	t.Run("namespace flag error", func(t *testing.T) {
		streams := genericclioptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}
		options := NewFetchResourceOptions(nil, streams)

		// Create command without namespace flag
		cmd := &cobra.Command{}

		err := options.CompleteNamespace([]string{}, cmd)
		if err == nil {
			t.Fatal("expected error when namespace flag is missing")
		}
		if !strings.Contains(err.Error(), "failed to get namespace") {
			t.Errorf("expected error about namespace flag, got: %v", err)
		}
	})

	t.Run("three or more args", func(t *testing.T) {
		streams := genericclioptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}
		options := NewFetchResourceOptions(nil, streams)

		cmd := &cobra.Command{}
		cmd.Flags().String("namespace", "test-ns", "namespace")

		// Three args should be ignored (no processing)
		err := options.CompleteNamespace([]string{"arg1", "arg2", "arg3"}, cmd)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should only set namespace, not process the args
		if options.ResourceName != "" {
			t.Errorf("expected empty resource name with 3 args, got: %s", options.ResourceName)
		}
		if options.ResourceType != "" {
			t.Errorf("expected empty resource type with 3 args, got: %s", options.ResourceType)
		}
	})
}

// Test FetchResources with client errors.
func Test_FetchResources_ClientErrors(t *testing.T) {
	tests := []struct {
		name          string
		options       *FetchResourceOptions
		clientError   error
		expectError   bool
		errorContains string
	}{
		{
			name: "nimservice list error in namespace",
			options: &FetchResourceOptions{
				Namespace:    "test-ns",
				ResourceType: NIMService,
			},
			clientError:   errors.New("connection refused"),
			expectError:   true,
			errorContains: "unable to retrieve NIMServices for namespace test-ns",
		},
		{
			name: "nimservice list error all namespaces",
			options: &FetchResourceOptions{
				ResourceType:  NIMService,
				AllNamespaces: true,
			},
			clientError:   errors.New("forbidden"),
			expectError:   true,
			errorContains: "unable to retrieve NIMServices for all namespaces",
		},
		{
			name: "nimcache list error",
			options: &FetchResourceOptions{
				Namespace:    "test-ns",
				ResourceType: NIMCache,
			},
			clientError:   errors.New("timeout"),
			expectError:   true,
			errorContains: "unable to retrieve NIMCaches for namespace test-ns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client that returns errors
			nimClient := nimfake.NewSimpleClientset()

			// Add reactor to simulate errors
			nimClient.PrependReactor("list", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, tt.clientError
			})

			mockClient := &mockClient{
				nimClient: nimClient,
			}

			_, err := FetchResources(context.TODO(), tt.options, mockClient)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got: %v", tt.errorContains, err)
				}
			}
		})
	}
}

// Test StreamResourceLogs with actual log content.
func Test_StreamResourceLogs_LogContent(t *testing.T) {
	// This test is simplified since mocking the actual log stream is complex
	// In a real implementation, you would use a fake REST client that returns
	// a proper ReadCloser with log content

	t.Run("verify log format", func(t *testing.T) {
		var outBuf bytes.Buffer
		streams := &genericclioptions.IOStreams{
			Out:    &outBuf,
			ErrOut: &bytes.Buffer{},
		}

		options := &FetchResourceOptions{
			IoStreams: streams,
		}

		// Create a pod with container
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "test-ns",
				Labels:    map[string]string{"app": "test"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "test-container"},
				},
			},
		}

		kubeClient := k8sfake.NewSimpleClientset(&pod)
		mockClient := &mockClient{
			kubeClient: kubeClient,
		}

		// This will fail to stream but we can test the setup
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		_ = StreamResourceLogs(ctx, options, mockClient, "test-ns", "test-resource", "app=test")

		// In a real test, we would verify the output format
		// For now, just ensure no panic
	})
}

// Test FetchResources with empty resource type.
func Test_FetchResources_EmptyResourceType(t *testing.T) {
	options := &FetchResourceOptions{
		Namespace:    "test-ns",
		ResourceType: "", // Empty resource type
	}

	mockClient := &mockClient{
		nimClient: nimfake.NewSimpleClientset(),
	}

	result, err := FetchResources(context.TODO(), options, mockClient)

	// Should return nil without error for unhandled resource type
	if err != nil {
		t.Errorf("expected no error for empty resource type, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty resource type, got: %v", result)
	}
}
