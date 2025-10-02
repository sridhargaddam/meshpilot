package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// AppStatus represents the status of a sample application
type AppStatus struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Deployed  bool     `json:"deployed"`
	Ready     bool     `json:"ready"`
	Replicas  int32    `json:"replicas"`
	Available int32    `json:"available"`
	Issues    []string `json:"issues,omitempty"`
}

// DeploySleepApp deploys the sleep sample application
func (m *Manager) DeploySleepApp(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		Namespace      string `json:"namespace,omitempty"`       // default: default
		IstioInjection bool   `json:"istio_injection,omitempty"` // default: true
		Replicas       int32  `json:"replicas,omitempty"`        // default: 1
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Invalid parameters: %v", err),
				},
			},
		}, nil
	}

	// Set defaults
	if params.Namespace == "" {
		params.Namespace = "default"
	}
	if params.Replicas == 0 {
		params.Replicas = 1
	}
	params.IstioInjection = true // Always enable for mesh testing

	ctx := context.Background()

	// Create namespace if it doesn't exist and enable Istio injection
	if err := m.createOrUpdateNamespace(ctx, params.Namespace, params.IstioInjection); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to create/update namespace: %v", err),
				},
			},
		}, nil
	}

	// Create ServiceAccount
	if err := m.createSleepServiceAccount(ctx, params.Namespace); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to create service account: %v", err),
				},
			},
		}, nil
	}

	// Create Deployment
	if err := m.createSleepDeployment(ctx, params.Namespace, params.Replicas); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to create deployment: %v", err),
				},
			},
		}, nil
	}

	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: fmt.Sprintf("Sleep app deployment initiated in namespace '%s' with %d replicas and Istio injection enabled", params.Namespace, params.Replicas),
			},
		},
	}, nil
}

// DeployHttpbinApp deploys the httpbin sample application
func (m *Manager) DeployHttpbinApp(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		Namespace      string `json:"namespace,omitempty"`       // default: default
		IstioInjection bool   `json:"istio_injection,omitempty"` // default: true
		Replicas       int32  `json:"replicas,omitempty"`        // default: 1
		ExposeService  bool   `json:"expose_service,omitempty"`  // default: true
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Invalid parameters: %v", err),
				},
			},
		}, nil
	}

	// Set defaults
	if params.Namespace == "" {
		params.Namespace = "default"
	}
	if params.Replicas == 0 {
		params.Replicas = 1
	}
	params.IstioInjection = true // Always enable for mesh testing
	params.ExposeService = true  // Always expose for testing

	ctx := context.Background()

	// Create namespace if it doesn't exist and enable Istio injection
	if err := m.createOrUpdateNamespace(ctx, params.Namespace, params.IstioInjection); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to create/update namespace: %v", err),
				},
			},
		}, nil
	}

	// Create ServiceAccount
	if err := m.createHttpbinServiceAccount(ctx, params.Namespace); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to create service account: %v", err),
				},
			},
		}, nil
	}

	// Create Deployment
	if err := m.createHttpbinDeployment(ctx, params.Namespace, params.Replicas); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to create deployment: %v", err),
				},
			},
		}, nil
	}

	// Create Service
	if params.ExposeService {
		if err := m.createHttpbinService(ctx, params.Namespace); err != nil {
			return &CallToolResult{
				IsError: true,
				Content: []interface{}{
					TextContent{
						Type: "text",
						Text: fmt.Sprintf("Failed to create service: %v", err),
					},
				},
			}, nil
		}
	}

	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: fmt.Sprintf("Httpbin app deployment initiated in namespace '%s' with %d replicas, Istio injection enabled, and service exposed", params.Namespace, params.Replicas),
			},
		},
	}, nil
}

// UndeploySleepApp removes the sleep sample application
func (m *Manager) UndeploySleepApp(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		Namespace string `json:"namespace,omitempty"` // default: default
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Invalid parameters: %v", err),
				},
			},
		}, nil
	}

	if params.Namespace == "" {
		params.Namespace = "default"
	}

	ctx := context.Background()

	// Delete deployment
	err := m.k8sClient.Kubernetes.AppsV1().Deployments(params.Namespace).Delete(ctx, "sleep", metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		logrus.Warnf("Failed to delete sleep deployment: %v", err)
	}

	// Delete service account
	err = m.k8sClient.Kubernetes.CoreV1().ServiceAccounts(params.Namespace).Delete(ctx, "sleep", metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		logrus.Warnf("Failed to delete sleep service account: %v", err)
	}

	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: fmt.Sprintf("Sleep app removal initiated from namespace '%s'", params.Namespace),
			},
		},
	}, nil
}

// UndeployHttpbinApp removes the httpbin sample application
func (m *Manager) UndeployHttpbinApp(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		Namespace string `json:"namespace,omitempty"` // default: default
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Invalid parameters: %v", err),
				},
			},
		}, nil
	}

	if params.Namespace == "" {
		params.Namespace = "default"
	}

	ctx := context.Background()

	// Delete deployment
	err := m.k8sClient.Kubernetes.AppsV1().Deployments(params.Namespace).Delete(ctx, "httpbin", metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		logrus.Warnf("Failed to delete httpbin deployment: %v", err)
	}

	// Delete service
	err = m.k8sClient.Kubernetes.CoreV1().Services(params.Namespace).Delete(ctx, "httpbin", metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		logrus.Warnf("Failed to delete httpbin service: %v", err)
	}

	// Delete service account
	err = m.k8sClient.Kubernetes.CoreV1().ServiceAccounts(params.Namespace).Delete(ctx, "httpbin", metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		logrus.Warnf("Failed to delete httpbin service account: %v", err)
	}

	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: fmt.Sprintf("Httpbin app removal initiated from namespace '%s'", params.Namespace),
			},
		},
	}, nil
}

// Helper functions for creating resources

func (m *Manager) createOrUpdateNamespace(ctx context.Context, name string, istioInjection bool) error {
	labels := map[string]string{}
	if istioInjection {
		labels["istio-injection"] = "enabled"
	}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}

	_, err := m.k8sClient.Kubernetes.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		// Update existing namespace with labels
		existingNs, getErr := m.k8sClient.Kubernetes.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}

		if existingNs.Labels == nil {
			existingNs.Labels = make(map[string]string)
		}

		for k, v := range labels {
			existingNs.Labels[k] = v
		}

		_, err = m.k8sClient.Kubernetes.CoreV1().Namespaces().Update(ctx, existingNs, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update namespace labels: %w", err)
		}
		return nil
	}

	return err
}

func (m *Manager) createSleepServiceAccount(ctx context.Context, namespace string) error {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sleep",
			Namespace: namespace,
			Labels: map[string]string{
				"app":     "sleep",
				"version": "v1",
			},
		},
	}

	_, err := m.k8sClient.Kubernetes.CoreV1().ServiceAccounts(namespace).Create(ctx, serviceAccount, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create service account: %w", err)
	}

	return nil
}

func (m *Manager) createSleepDeployment(ctx context.Context, namespace string, replicas int32) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sleep",
			Namespace: namespace,
			Labels: map[string]string{
				"app":     "sleep",
				"version": "v1",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "sleep",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":     "sleep",
						"version": "v1",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "sleep",
					Containers: []corev1.Container{
						{
							Name:  "sleep",
							Image: "curlimages/curl:8.5.0",
							Command: []string{
								"/bin/sleep",
								"infinity",
							},
							ImagePullPolicy: corev1.PullIfNotPresent,
							VolumeMounts: []corev1.VolumeMount{
								{
									MountPath: "/etc/sleep/tls",
									Name:      "secret-volume",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("32Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "secret-volume",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "sleep-secret",
									Optional:   boolPtr(true),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := m.k8sClient.Kubernetes.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	return nil
}

func (m *Manager) createHttpbinServiceAccount(ctx context.Context, namespace string) error {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin",
			Namespace: namespace,
			Labels: map[string]string{
				"app":     "httpbin",
				"version": "v1",
			},
		},
	}

	_, err := m.k8sClient.Kubernetes.CoreV1().ServiceAccounts(namespace).Create(ctx, serviceAccount, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create service account: %w", err)
	}

	return nil
}

func (m *Manager) createHttpbinDeployment(ctx context.Context, namespace string, replicas int32) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin",
			Namespace: namespace,
			Labels: map[string]string{
				"app":     "httpbin",
				"version": "v1",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "httpbin",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":     "httpbin",
						"version": "v1",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "httpbin",
					Containers: []corev1.Container{
						{
							Name:            "httpbin",
							Image:           "quay.io/sridhargaddam/kong/httpbin:latest",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command: []string{
								"gunicorn",
								"-b",
								"[::]:8000",
								"httpbin:app",
								"-k",
								"gevent",
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8000,
									Name:          "http",
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := m.k8sClient.Kubernetes.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	return nil
}

func (m *Manager) createHttpbinService(ctx context.Context, namespace string) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin",
			Namespace: namespace,
			Labels: map[string]string{
				"app":     "httpbin",
				"service": "httpbin",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8000,
					TargetPort: intstr.FromInt(8000),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": "httpbin",
			},
		},
	}

	_, err := m.k8sClient.Kubernetes.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create service: %w", err)
	}

	return nil
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}
