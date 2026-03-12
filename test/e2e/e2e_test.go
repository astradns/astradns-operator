//go:build e2e
// +build e2e

/*
Copyright 2026.

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

package e2e

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/astradns/astradns-operator/test/utils"
)

// namespace where the project is deployed in
const namespace = "astradns-operator-system"

// serviceAccountName created for the project
const serviceAccountName = "astradns-operator-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "astradns-operator-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "astradns-operator-metrics-binding"

// metricsReaderRoleName is the prefixed ClusterRole created by kustomize.
const metricsReaderRoleName = "astradns-operator-metrics-reader"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", managerImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("undeploying the controller-manager")
		cmd := exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching metrics endpoint output")
			metricsOutput, err := getMetricsOutput()
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics output:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get metrics output: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			Expect(ensureMetricsReaderBinding()).To(Succeed())

			By("validating that the metrics service is available")
			cmd := exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("ensuring the controller pod is ready")
			verifyControllerPodReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", controllerPodName, "-n", namespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Controller pod not ready")
			}
			Eventually(verifyControllerPodReady, 3*time.Minute, time.Second).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("Serving metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted, 3*time.Minute, time.Second).Should(Succeed())

			// +kubebuilder:scaffold:e2e-metrics-webhooks-readiness

			By("getting metrics from a fresh curl pod")
			verifyMetricsAvailable := func(g Gomega) {
				metricsOutput, err := getMetricsOutput()
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve metrics output")
				g.Expect(metricsOutput).NotTo(BeEmpty())
				g.Expect(metricsOutput).To(ContainSubstring("controller_runtime_reconcile_total"))
			}
			Eventually(verifyMetricsAvailable, 2*time.Minute).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		It("should create ConfigMap with correct JSON when a DNSUpstreamPool is applied", func() {
			poolName := "test-pool-configmap"

			By("applying a DNSUpstreamPool CR")
			poolYAML := fmt.Sprintf(`apiVersion: dns.astradns.com/v1alpha1
kind: DNSUpstreamPool
metadata:
  name: %s
  namespace: %s
spec:
  upstreams:
  - address: "1.1.1.1"
    port: 53
  - address: "8.8.8.8"
    port: 53
  healthCheck:
    enabled: true
    intervalSeconds: 30
    timeoutSeconds: 5
    failureThreshold: 3
  loadBalancing:
    strategy: round-robin`, poolName, namespace)
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(poolYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply DNSUpstreamPool CR")

			By("verifying ConfigMap astradns-agent-config exists with config.json key")
			verifyConfigMap := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "configmap", "astradns-agent-config",
					"-n", namespace,
					"-o", "jsonpath={.data.config\\.json}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "ConfigMap astradns-agent-config should exist")
				g.Expect(output).NotTo(BeEmpty(), "config.json key should not be empty")

				// Verify the JSON contains the upstream addresses
				g.Expect(output).To(ContainSubstring("1.1.1.1"),
					"config.json should contain upstream address 1.1.1.1")
				g.Expect(output).To(ContainSubstring("8.8.8.8"),
					"config.json should contain upstream address 8.8.8.8")

				// Verify the JSON is valid
				var config map[string]interface{}
				g.Expect(json.Unmarshal([]byte(output), &config)).To(Succeed(),
					"config.json should be valid JSON")
			}
			Eventually(verifyConfigMap, time.Minute).Should(Succeed())

			By("verifying the pool status has Ready=True condition")
			verifyPoolReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					fmt.Sprintf("dnsupstreampools.dns.astradns.com/%s", poolName),
					"-n", namespace,
					"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "DNSUpstreamPool should have Ready=True condition")
			}
			Eventually(verifyPoolReady, time.Minute).Should(Succeed())

			By("cleaning up the DNSUpstreamPool")
			cmd = exec.Command("kubectl", "delete",
				fmt.Sprintf("dnsupstreampools.dns.astradns.com/%s", poolName),
				"-n", namespace, "--ignore-not-found")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete DNSUpstreamPool")
		})

		It("should include cache configuration when DNSCacheProfile and DNSUpstreamPool are applied together", func() {
			profileName := "default"
			poolName := "test-pool-cache"

			By("applying a DNSCacheProfile named 'default'")
			profileYAML := fmt.Sprintf(`apiVersion: dns.astradns.com/v1alpha1
kind: DNSCacheProfile
metadata:
  name: %s
  namespace: %s
spec:
  maxEntries: 100000
  positiveTtl:
    minSeconds: 60
    maxSeconds: 300
  negativeTtl:
    seconds: 30
  prefetch:
    enabled: true
    threshold: 10`, profileName, namespace)
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(profileYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply DNSCacheProfile CR")

			By("applying a DNSUpstreamPool CR")
			poolYAML := fmt.Sprintf(`apiVersion: dns.astradns.com/v1alpha1
kind: DNSUpstreamPool
metadata:
  name: %s
  namespace: %s
spec:
  upstreams:
  - address: "1.1.1.1"
    port: 53
  healthCheck:
    enabled: true
    intervalSeconds: 30
  loadBalancing:
    strategy: round-robin`, poolName, namespace)
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(poolYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply DNSUpstreamPool CR")

			By("verifying ConfigMap contains cache configuration from the profile")
			verifyConfigWithCache := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "configmap", "astradns-agent-config",
					"-n", namespace,
					"-o", "jsonpath={.data.config\\.json}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "ConfigMap astradns-agent-config should exist")
				g.Expect(output).NotTo(BeEmpty(), "config.json key should not be empty")

				// Parse the JSON to verify cache fields are populated from the profile
				var config map[string]interface{}
				g.Expect(json.Unmarshal([]byte(output), &config)).To(Succeed(),
					"config.json should be valid JSON")

				cacheRaw, hasCacheKey := config["cache"]
				g.Expect(hasCacheKey).To(BeTrue(), "config.json should contain a 'cache' section")

				cacheMap, ok := cacheRaw.(map[string]interface{})
				g.Expect(ok).To(BeTrue(), "'cache' should be a JSON object")

				// Verify cache values match the DNSCacheProfile spec
				g.Expect(cacheMap["maxEntries"]).To(BeNumerically("==", 100000),
					"maxEntries should match DNSCacheProfile spec")
				g.Expect(cacheMap["positiveTtlMin"]).To(BeNumerically("==", 60),
					"positiveTtlMin should match DNSCacheProfile spec")
				g.Expect(cacheMap["positiveTtlMax"]).To(BeNumerically("==", 300),
					"positiveTtlMax should match DNSCacheProfile spec")
				g.Expect(cacheMap["negativeTtl"]).To(BeNumerically("==", 30),
					"negativeTtl should match DNSCacheProfile spec")
				g.Expect(cacheMap["prefetchEnabled"]).To(BeTrue(),
					"prefetchEnabled should match DNSCacheProfile spec")
				g.Expect(cacheMap["prefetchThreshold"]).To(BeNumerically("==", 10),
					"prefetchThreshold should match DNSCacheProfile spec")
			}
			Eventually(verifyConfigWithCache, time.Minute).Should(Succeed())

			By("cleaning up the DNSUpstreamPool and DNSCacheProfile")
			cmd = exec.Command("kubectl", "delete",
				fmt.Sprintf("dnsupstreampools.dns.astradns.com/%s", poolName),
				"-n", namespace, "--ignore-not-found")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete DNSUpstreamPool")

			cmd = exec.Command("kubectl", "delete",
				fmt.Sprintf("dnscacheprofiles.dns.astradns.com/%s", profileName),
				"-n", namespace, "--ignore-not-found")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete DNSCacheProfile")
		})

		It("should remove config.json key from ConfigMap when the pool is deleted", func() {
			poolName := "test-pool-deletion"

			By("applying a DNSUpstreamPool CR")
			poolYAML := fmt.Sprintf(`apiVersion: dns.astradns.com/v1alpha1
kind: DNSUpstreamPool
metadata:
  name: %s
  namespace: %s
spec:
  upstreams:
  - address: "9.9.9.9"
    port: 53
  healthCheck:
    enabled: true
  loadBalancing:
    strategy: round-robin`, poolName, namespace)
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(poolYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply DNSUpstreamPool CR")

			By("waiting for ConfigMap to appear with config.json")
			verifyConfigExists := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "configmap", "astradns-agent-config",
					"-n", namespace,
					"-o", "jsonpath={.data.config\\.json}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "ConfigMap should exist")
				g.Expect(output).NotTo(BeEmpty(), "config.json should not be empty")
			}
			Eventually(verifyConfigExists, time.Minute).Should(Succeed())

			By("deleting the DNSUpstreamPool")
			cmd = exec.Command("kubectl", "delete",
				fmt.Sprintf("dnsupstreampools.dns.astradns.com/%s", poolName),
				"-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete DNSUpstreamPool")

			By("verifying ConfigMap still exists but config.json key is removed")
			verifyConfigKeyRemoved := func(g Gomega) {
				// Verify ConfigMap still exists
				cmd := exec.Command("kubectl", "get", "configmap", "astradns-agent-config",
					"-n", namespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "ConfigMap should still exist after pool deletion")

				// Verify config.json key is removed (empty output means key does not exist)
				cmd = exec.Command("kubectl", "get", "configmap", "astradns-agent-config",
					"-n", namespace,
					"-o", "jsonpath={.data.config\\.json}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(BeEmpty(),
					"config.json key should be removed after pool deletion")
			}
			Eventually(verifyConfigKeyRemoved, time.Minute).Should(Succeed())
		})

		It("should reject pool creation when upstreams list is empty", func() {
			poolName := "test-pool-invalid"

			By("applying a DNSUpstreamPool with empty upstreams list")
			poolYAML := fmt.Sprintf(`apiVersion: dns.astradns.com/v1alpha1
kind: DNSUpstreamPool
metadata:
  name: %s
  namespace: %s
spec:
  upstreams: []
  healthCheck:
    enabled: true
  loadBalancing:
    strategy: round-robin`, poolName, namespace)
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(poolYAML)
			_, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Applying invalid DNSUpstreamPool CR should fail")
			Expect(err.Error()).To(ContainSubstring("spec.upstreams"))
		})

		It("should record successful reconciliation metrics for DNSUpstreamPool", func() {
			poolName := "test-pool-metrics"

			By("applying a DNSUpstreamPool CR")
			poolYAML := fmt.Sprintf(`apiVersion: dns.astradns.com/v1alpha1
kind: DNSUpstreamPool
metadata:
  name: %s
  namespace: %s
spec:
  upstreams:
  - address: "1.0.0.1"
    port: 53
  healthCheck:
    enabled: true
  loadBalancing:
    strategy: round-robin`, poolName, namespace)
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(poolYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply DNSUpstreamPool CR")

			By("waiting for reconciliation to complete (pool becomes Ready)")
			verifyPoolReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					fmt.Sprintf("dnsupstreampools.dns.astradns.com/%s", poolName),
					"-n", namespace,
					"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Pool should be Ready before checking metrics")
			}
			Eventually(verifyPoolReady, time.Minute).Should(Succeed())

			By("verifying reconciliation metrics contain successful dnsupstreampool reconcile count")
			verifyReconcileMetrics := func(g Gomega) {
				metricsOutput, err := getMetricsOutput()
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve metrics output")
				g.Expect(metricsOutput).To(ContainSubstring(
					`controller_runtime_reconcile_total{controller="dnsupstreampool",result="success"}`),
					"Metrics should contain successful reconcile count for dnsupstreampool controller")
			}
			Eventually(verifyReconcileMetrics, 2*time.Minute).Should(Succeed())

			By("cleaning up the DNSUpstreamPool")
			cmd = exec.Command("kubectl", "delete",
				fmt.Sprintf("dnsupstreampools.dns.astradns.com/%s", poolName),
				"-n", namespace, "--ignore-not-found")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete DNSUpstreamPool")
		})
	})
})

// serviceAccountToken returns a token for the operator service account.
func serviceAccountToken() (string, error) {
	cmd := exec.Command(
		"kubectl",
		"create",
		"token",
		serviceAccountName,
		"-n",
		namespace,
		"--duration=10m",
	)

	output, err := utils.Run(cmd)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(output), nil
}

// ensureMetricsReaderBinding creates/updates the ClusterRoleBinding needed to read /metrics.
func ensureMetricsReaderBinding() error {
	binding := fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: %s
subjects:
- kind: ServiceAccount
  name: %s
  namespace: %s
`, metricsRoleBindingName, metricsReaderRoleName, serviceAccountName, namespace)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(binding)
	_, err := utils.Run(cmd)
	return err
}

// getMetricsOutput retrieves metrics through the Kubernetes API service proxy.
func getMetricsOutput() (string, error) {
	token, err := serviceAccountToken()
	if err != nil {
		return "", err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	localPort := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	portForwardCmd := exec.Command(
		"kubectl",
		"port-forward",
		"-n",
		namespace,
		fmt.Sprintf("service/%s", metricsServiceName),
		fmt.Sprintf("%d:8443", localPort),
	)
	if err := portForwardCmd.Start(); err != nil {
		return "", err
	}
	defer func() {
		_ = portForwardCmd.Process.Kill()
		_ = portForwardCmd.Wait()
	}()

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/metrics", localPort)
	deadline := time.Now().Add(20 * time.Second)
	var errs []string

	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

		resp, err := client.Do(req)
		if err != nil {
			errs = append(errs, err.Error())
			time.Sleep(300 * time.Millisecond)
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return "", readErr
		}

		if resp.StatusCode == http.StatusOK {
			return string(body), nil
		}

		errs = append(errs, fmt.Sprintf("status %d: %s", resp.StatusCode, string(body)))
		time.Sleep(300 * time.Millisecond)
	}

	return "", fmt.Errorf("failed to retrieve metrics through port-forward: %s", strings.Join(errs, "; "))
}
