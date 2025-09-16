package util

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"k8s-nim-operator-cli/pkg/util/client"

	apimeta "k8s.io/apimachinery/pkg/api/meta"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"

	"bufio"
)

type FetchResourceOptions struct {
	cmdFactory    cmdutil.Factory
	IoStreams     *genericclioptions.IOStreams
	Namespace     string
	ResourceName  string
	ResourceType  ResourceType
	AllNamespaces bool
}

func NewFetchResourceOptions(cmdFactory cmdutil.Factory, streams genericclioptions.IOStreams) *FetchResourceOptions {
	return &FetchResourceOptions{
		cmdFactory: cmdFactory,
		IoStreams:  &streams,
	}
}

// Populates FetchResourceOptions with namespace and resource name (if present).
func (options *FetchResourceOptions) CompleteNamespace(args []string, cmd *cobra.Command) error {
	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return fmt.Errorf("failed to get namespace: %w", err)
	}
	options.Namespace = namespace
	if options.Namespace == "" {
		options.Namespace = "default"
	}

	// When get and status call this, there will only ever be one argument at most (nim get NIMSERVICE NAME or nim get NIMSERVICES).
	// When logs calls this command, ResourceName will immediately be overwritten by a blank string.
	if len(args) == 1 {
		options.ResourceName = args[0]
	}
	// There would be exactly two arguments if delete calls this (nim DELETE NIMSERVICE META-LLAMA-3B).
	if len(args) == 2 {
		resourceType := ResourceType(strings.ToLower(args[0]))

		// Validating ResourceType.
		switch resourceType {
		case NIMService, NIMCache:
			options.ResourceType = resourceType
		default:
			return fmt.Errorf("invalid resource type %q. Valid types are: nimservice, nimcache", args[0])
		}

		options.ResourceName = args[1]
	}

	return nil
}

// Returns list of matching resources.
func FetchResources(ctx context.Context, options *FetchResourceOptions, k8sClient client.Client) (interface{}, error) {
	var resourceList interface{}
	var err error

	listopts := v1.ListOptions{}
	if options.ResourceName != "" {
		listopts = v1.ListOptions{
			FieldSelector: fmt.Sprintf("metadata.name=%s", options.ResourceName),
		}
	}

	switch options.ResourceType {

	case NIMService:
		resourceList = appsv1alpha1.NIMServiceList{}

		// Retrieve NIMServices.
		if options.AllNamespaces {
			resourceList, err = k8sClient.NIMClient().AppsV1alpha1().NIMServices("").List(ctx, listopts)
			if err != nil {
				return nil, fmt.Errorf("unable to retrieve NIMServices for all namespaces: %w", err)
			}
		} else {
			resourceList, err = k8sClient.NIMClient().AppsV1alpha1().NIMServices(options.Namespace).List(ctx, listopts)
			if err != nil {
				return nil, fmt.Errorf("unable to retrieve NIMServices for namespace %s: %w", options.Namespace, err)
			}
		}

		// Cast resourceList to NIMServiceList.
		nimServiceList, ok := resourceList.(*appsv1alpha1.NIMServiceList)
		if !ok {
			return nil, fmt.Errorf("failed to cast resourceList to NIMServiceList")
		}

		if options.ResourceName != "" && len(nimServiceList.Items) == 0 {
			errMsg := fmt.Sprintf("NIMService %s not found", options.ResourceName)
			if options.AllNamespaces {
				errMsg += " in any namespace"
			} else {
				errMsg += fmt.Sprintf(" in namespace %s", options.Namespace)
			}
			return nil, errors.New(errMsg)
		}

	case NIMCache:
		resourceList = appsv1alpha1.NIMCacheList{}

		// Retrieve NIMCaches.
		if options.AllNamespaces {
			resourceList, err = k8sClient.NIMClient().AppsV1alpha1().NIMCaches("").List(ctx, listopts)
			if err != nil {
				return nil, fmt.Errorf("unable to retrieve NIMCaches for all namespaces: %w", err)
			}
		} else {
			resourceList, err = k8sClient.NIMClient().AppsV1alpha1().NIMCaches(options.Namespace).List(ctx, listopts)
			if err != nil {
				return nil, fmt.Errorf("unable to retrieve NIMCaches for namespace %s: %w", options.Namespace, err)
			}
		}

		nimCacheList, ok := resourceList.(*appsv1alpha1.NIMCacheList)
		if !ok {
			return nil, fmt.Errorf("failed to cast resourceList to NIMCacheList")
		}

		if options.ResourceName != "" && len(nimCacheList.Items) == 0 {
			errMsg := fmt.Sprintf("NIMCache %s not found", options.ResourceName)
			if options.AllNamespaces {
				errMsg += " in any namespace"
			} else {
				errMsg += fmt.Sprintf(" in namespace %s", options.Namespace)
			}
			return nil, errors.New(errMsg)
		}

	}

	return resourceList, nil
}

func messageConditionFrom(conds []v1.Condition) (*v1.Condition, error) {
	// Prefer a Failed with a non-empty message
	if failed := apimeta.FindStatusCondition(conds, "Failed"); failed != nil && failed.Message != "" {
		return failed, nil
	}
	// Fallback to Ready if present (message may be empty)
	if ready := apimeta.FindStatusCondition(conds, "Ready"); ready != nil {
		return ready, nil
	}
	// Otherwise: first condition with a non-empty message
	for i := range conds {
		if conds[i].Message != "" {
			return &conds[i], nil
		}
	}
	if len(conds) > 0 {
		// Last resort: return the first condition even if it has no message
		return &conds[0], nil
	}
	return nil, fmt.Errorf("no conditions present")
}

func MessageCondition(obj interface{}) (*v1.Condition, error) {
	switch t := obj.(type) {
	case *appsv1alpha1.NIMCache:
		return messageConditionFrom(t.Status.Conditions)
	case *appsv1alpha1.NIMService:
		return messageConditionFrom(t.Status.Conditions)
	default:
		return nil, fmt.Errorf("unsupported type %T (want *NIMCache or *NIMService)", obj)
	}
}

// StreamResourceEvents lists pods by label selector and prints logs related to those pods.
func StreamResourceLogs(ctx context.Context, options *FetchResourceOptions, k8sClient client.Client, namespace string, resourceName string, labelSelector string) error {
	kube := k8sClient.KubernetesClient()
	pods, err := kube.CoreV1().Pods(namespace).List(ctx, v1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return fmt.Errorf("no pods found for %s/%s (selector=%q)", namespace, resourceName, labelSelector)
	}

	type logLine struct {
		pod, container string
		text           string
	}
	lines := make(chan logLine, 1024)

	// Flags (replace with real flags)
	follow := true
	allContainers := true
	targetContainer := ""
	timestamps := false

	var wg sync.WaitGroup
	for _, pod := range pods.Items {
		containers := pod.Spec.Containers
		if !allContainers && targetContainer != "" {
			containers = []corev1.Container{{Name: targetContainer}}
		}
		for _, c := range containers {
			wg.Add(1)
			podName, containerName := pod.Name, c.Name
			go func() {
				defer wg.Done()
				req := kube.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
					Container:  containerName,
					Follow:     follow,
					Timestamps: timestamps,
				})
				rc, err := req.Stream(ctx)
				if err != nil {
					fmt.Fprintf(options.IoStreams.ErrOut, "error streaming %s/%s[%s]: %v\n", namespace, podName, containerName, err)
					return
				}
				defer rc.Close()

				sc := bufio.NewScanner(rc)
				for sc.Scan() {
					select {
					case <-ctx.Done():
						return
					case lines <- logLine{pod: podName, container: containerName, text: sc.Text()}:
					}
				}
			}()
		}
	}

	// Close channel when all streams end
	go func() {
		wg.Wait()
		close(lines)
	}()

	// Printer: interleaves lines as they arrive
	for ln := range lines {
		fmt.Fprintf(options.IoStreams.Out, "[%s/%s] %s\n", ln.pod, ln.container, ln.text)
	}
	return nil
}