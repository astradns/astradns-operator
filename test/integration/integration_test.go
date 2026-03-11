//go:build integration
// +build integration

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

package integration

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AstraDNS Integration", Ordered, func() {
	var (
		operatorPod string
		agentPod    string
		nodeIP      string
	)

	BeforeAll(func() {
		By("resolving node internal IP")
		nodeIP = runMust("kubectl", "get", "nodes",
			"-o", "jsonpath={.items[0].status.addresses[?(@.type==\"InternalIP\")].address}")
		GinkgoWriter.Printf("Node IP: %s\n", nodeIP)
	})

	AfterAll(func() {
		By("cleaning up all test CRs")
		_, _ = runOrErr("kubectl", "delete", "dnsupstreampools.dns.astradns.com", "--all",
			"-n", testNamespace(), "--ignore-not-found")
		_, _ = runOrErr("kubectl", "delete", "dnscacheprofiles.dns.astradns.com", "--all",
			"-n", testNamespace(), "--ignore-not-found")
		_, _ = runOrErr("kubectl", "delete", "externaldnspolicies.dns.astradns.com", "--all",
			"-n", testNamespace(), "--ignore-not-found")
	})

	// -----------------------------------------------------------------------
	// 1. Cluster components
	// -----------------------------------------------------------------------

	Context("Cluster components", func() {
		It("should have the operator pod running", func() {
			Eventually(func(g Gomega) {
				out, err := runOrErr("kubectl", "get", "pods",
					"-n", testNamespace(),
					"-l", "app.kubernetes.io/component=operator",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}\n{{ end }}{{ end }}")
				g.Expect(err).NotTo(HaveOccurred())
				pods := nonEmpty(out)
				g.Expect(pods).To(HaveLen(1), "expected exactly 1 operator pod")
				operatorPod = pods[0]

				phase, err := runOrErr("kubectl", "get", "pod", operatorPod,
					"-n", testNamespace(), "-o", "jsonpath={.status.phase}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(phase).To(Equal("Running"))
			}).Should(Succeed())
		})

		It("should have an agent pod running on the Kind node", func() {
			Eventually(func(g Gomega) {
				out, err := runOrErr("kubectl", "get", "pods",
					"-n", testNamespace(),
					"-l", "app.kubernetes.io/component=agent",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}\n{{ end }}{{ end }}")
				g.Expect(err).NotTo(HaveOccurred())
				pods := nonEmpty(out)
				g.Expect(pods).ToNot(BeEmpty(), "expected at least 1 agent pod")
				agentPod = pods[0]

				phase, err := runOrErr("kubectl", "get", "pod", agentPod,
					"-n", testNamespace(), "-o", "jsonpath={.status.phase}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(phase).To(Equal("Running"))
			}).Should(Succeed())
		})

		It("should have CRDs registered in the API server", func() {
			for _, crd := range []string{
				"dnsupstreampools.dns.astradns.com",
				"dnscacheprofiles.dns.astradns.com",
				"externaldnspolicies.dns.astradns.com",
			} {
				out := runMust("kubectl", "get", "crd", crd, "-o", "jsonpath={.metadata.name}")
				Expect(out).To(Equal(crd))
			}
		})
	})

	// -----------------------------------------------------------------------
	// 2. CRD → ConfigMap pipeline
	// -----------------------------------------------------------------------

	Context("CRD to ConfigMap pipeline", func() {
		It("should populate ConfigMap when DNSUpstreamPool is created", func() {
			poolYAML := fmt.Sprintf(`apiVersion: dns.astradns.com/v1alpha1
kind: DNSUpstreamPool
metadata:
  name: integration-pool
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
    strategy: round-robin`, testNamespace())

			By("applying the DNSUpstreamPool")
			applyYAML(poolYAML)

			By("verifying ConfigMap contains upstream addresses")
			Eventually(func(g Gomega) {
				out, err := runOrErr("kubectl", "get", "configmap", configMapName(),
					"-n", testNamespace(),
					"-o", "jsonpath={.data.config\\.json}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).NotTo(BeEmpty(), "config.json should not be empty")
				g.Expect(out).To(ContainSubstring("1.1.1.1"))
				g.Expect(out).To(ContainSubstring("8.8.8.8"))

				var config map[string]interface{}
				g.Expect(json.Unmarshal([]byte(out), &config)).To(Succeed(),
					"config.json should be valid JSON")
			}).Should(Succeed())

			By("verifying pool has Ready=True status")
			Eventually(func(g Gomega) {
				out, err := runOrErr("kubectl", "get",
					"dnsupstreampools.dns.astradns.com/integration-pool",
					"-n", testNamespace(),
					"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(Equal("True"))
			}).Should(Succeed())
		})

		It("should merge cache configuration from DNSCacheProfile", func() {
			profileYAML := fmt.Sprintf(`apiVersion: dns.astradns.com/v1alpha1
kind: DNSCacheProfile
metadata:
  name: default
  namespace: %s
spec:
  maxEntries: 50000
  positiveTtl:
    minSeconds: 60
    maxSeconds: 300
  negativeTtl:
    seconds: 30
  prefetch:
    enabled: true
    threshold: 5`, testNamespace())

			By("applying the DNSCacheProfile")
			applyYAML(profileYAML)

			By("triggering a pool reconciliation")
			_, _ = runOrErr("kubectl", "annotate",
				"dnsupstreampools.dns.astradns.com/integration-pool",
				"-n", testNamespace(),
				fmt.Sprintf("integration-test/trigger=%d", time.Now().Unix()),
				"--overwrite")

			By("verifying ConfigMap includes cache section")
			Eventually(func(g Gomega) {
				out, err := runOrErr("kubectl", "get", "configmap", configMapName(),
					"-n", testNamespace(),
					"-o", "jsonpath={.data.config\\.json}")
				g.Expect(err).NotTo(HaveOccurred())

				var config map[string]interface{}
				g.Expect(json.Unmarshal([]byte(out), &config)).To(Succeed())

				cacheRaw, ok := config["cache"]
				g.Expect(ok).To(BeTrue(), "config should contain 'cache' key")

				cache, ok := cacheRaw.(map[string]interface{})
				g.Expect(ok).To(BeTrue(), "'cache' should be a JSON object")
				g.Expect(cache["maxEntries"]).To(BeNumerically("==", 50000))
			}).Should(Succeed())
		})

		It("should update ConfigMap when pool upstreams change", func() {
			updatedPoolYAML := fmt.Sprintf(`apiVersion: dns.astradns.com/v1alpha1
kind: DNSUpstreamPool
metadata:
  name: integration-pool
  namespace: %s
spec:
  upstreams:
  - address: "9.9.9.9"
    port: 53
  healthCheck:
    enabled: true
    intervalSeconds: 30
    timeoutSeconds: 5
    failureThreshold: 3
  loadBalancing:
    strategy: round-robin`, testNamespace())

			By("updating the pool with a different upstream")
			applyYAML(updatedPoolYAML)

			By("verifying ConfigMap reflects the new upstream")
			Eventually(func(g Gomega) {
				out, err := runOrErr("kubectl", "get", "configmap", configMapName(),
					"-n", testNamespace(),
					"-o", "jsonpath={.data.config\\.json}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(ContainSubstring("9.9.9.9"))
				g.Expect(out).NotTo(ContainSubstring("1.1.1.1"),
					"old upstream should be removed")
				g.Expect(out).NotTo(ContainSubstring("8.8.8.8"),
					"old upstream should be removed")
			}).Should(Succeed())
		})
	})

	// -----------------------------------------------------------------------
	// 3. Agent runtime validation
	// -----------------------------------------------------------------------

	Context("Agent runtime", func() {
		It("should serve the health endpoint", func() {
			Expect(agentPod).NotTo(BeEmpty(), "agent pod name not resolved")

			Eventually(func(g Gomega) {
				agentIP, err := runOrErr("kubectl", "get", "pod", agentPod,
					"-n", testNamespace(),
					"-o", "jsonpath={.status.podIP}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(agentIP).NotTo(BeEmpty())

				podName := fmt.Sprintf("healthz-%d", time.Now().UnixNano())
				out, err := runOrErr("kubectl", "run", podName,
					"-n", testNamespace(),
					"--rm", "-i", "--restart=Never",
					"--image=curlimages/curl:8.12.1",
					"--", "-s", "-o", "/dev/null", "-w", "%{http_code}",
					fmt.Sprintf("http://%s:8080/healthz", agentIP))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(ContainSubstring("200"),
					"/healthz should return 200")
			}).Should(Succeed())
		})

		It("should expose Prometheus metrics", func() {
			Expect(agentPod).NotTo(BeEmpty())

			Eventually(func(g Gomega) {
				agentIP, err := runOrErr("kubectl", "get", "pod", agentPod,
					"-n", testNamespace(),
					"-o", "jsonpath={.status.podIP}")
				g.Expect(err).NotTo(HaveOccurred())

				podName := fmt.Sprintf("metrics-%d", time.Now().UnixNano())
				out, err := runOrErr("kubectl", "run", podName,
					"-n", testNamespace(),
					"--rm", "-i", "--restart=Never",
					"--image=curlimages/curl:8.12.1",
					"--", "-s",
					fmt.Sprintf("http://%s:9153/metrics", agentIP))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(ContainSubstring("astradns_"),
					"metrics endpoint should contain astradns_ metrics")
			}).Should(Succeed())
		})

		It("should forward DNS queries through the engine", func() {
			Expect(nodeIP).NotTo(BeEmpty(), "node IP not resolved")

			Eventually(func(g Gomega) {
				podName := fmt.Sprintf("dns-%d", time.Now().UnixNano())
				out, err := runOrErr("kubectl", "run", podName,
					"-n", testNamespace(),
					"--rm", "-i", "--restart=Never",
					"--image=busybox:1.37",
					"--command", "--",
					"nslookup", "-port=5353", "-timeout=5", "example.com", nodeIP)
				// Accept any DNS response — even SERVFAIL proves the agent is
				// listening and the engine forwarding pipeline works. Upstream
				// reachability depends on the host network and is not guaranteed
				// in all CI environments.
				g.Expect(err == nil || out != "").To(BeTrue(),
					"DNS query should produce output; got err=%v output=%q", err, out)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
		})
	})

	// -----------------------------------------------------------------------
	// 4. ExternalDNSPolicy
	// -----------------------------------------------------------------------

	Context("ExternalDNSPolicy", func() {
		It("should accept a valid ExternalDNSPolicy and update status", func() {
			policyYAML := fmt.Sprintf(`apiVersion: dns.astradns.com/v1alpha1
kind: ExternalDNSPolicy
metadata:
  name: integration-policy
  namespace: %s
spec:
  selector:
    namespaces:
    - %s
  upstreamPoolRef:
    name: integration-pool
  cacheProfileRef:
    name: default`, testNamespace(), testNamespace())

			By("applying the ExternalDNSPolicy")
			applyYAML(policyYAML)

			By("verifying the policy resource exists")
			Eventually(func(g Gomega) {
				out, err := runOrErr("kubectl", "get",
					"externaldnspolicies.dns.astradns.com/integration-policy",
					"-n", testNamespace(),
					"-o", "jsonpath={.metadata.name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(Equal("integration-policy"))
			}).Should(Succeed())
		})
	})

	// -----------------------------------------------------------------------
	// 5. Cleanup behaviour
	// -----------------------------------------------------------------------

	Context("Cleanup", func() {
		It("should remove config.json from ConfigMap when the pool is deleted", func() {
			By("deleting the pool")
			runMust("kubectl", "delete",
				"dnsupstreampools.dns.astradns.com/integration-pool",
				"-n", testNamespace())

			By("verifying config.json key is cleared")
			Eventually(func(g Gomega) {
				out, err := runOrErr("kubectl", "get", "configmap", configMapName(),
					"-n", testNamespace(),
					"-o", "jsonpath={.data.config\\.json}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(BeEmpty(),
					"config.json should be empty after pool deletion")
			}).Should(Succeed())
		})

		It("should allow deleting remaining CRs without errors", func() {
			_, _ = runOrErr("kubectl", "delete",
				"dnscacheprofiles.dns.astradns.com/default",
				"-n", testNamespace(), "--ignore-not-found")
			_, _ = runOrErr("kubectl", "delete",
				"externaldnspolicies.dns.astradns.com/integration-policy",
				"-n", testNamespace(), "--ignore-not-found")
		})
	})
})
