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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	helmRelease = "astradns"
	chartPath   = "deploy/helm/astradns"
	operatorImg = "astradns-operator:integration"
	agentImg    = "astradns-agent:integration"
)

// kindClusterName returns a cluster name that includes the K8s version suffix
// when KIND_NODE_IMAGE is set, so multiple versions can run in parallel.
//
//	KIND_NODE_IMAGE=kindest/node:v1.31.6  →  astradns-integration-v1-31
//	(unset)                                →  astradns-integration
func kindClusterName() string {
	base := "astradns-integration"
	img := os.Getenv("KIND_NODE_IMAGE")
	if img == "" {
		return base
	}
	// Extract "v1.31" from "kindest/node:v1.31.6"
	if idx := strings.LastIndex(img, ":"); idx >= 0 {
		tag := img[idx+1:]
		parts := strings.SplitN(tag, ".", 3) // ["v1", "31", "6"]
		if len(parts) >= 2 {
			return fmt.Sprintf("%s-%s-%s", base, parts[0], parts[1])
		}
	}
	return base
}

// namespace derives from the cluster name so parallel runs don't collide.
func testNamespace() string {
	return kindClusterName()
}

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AstraDNS Integration Suite")
}

var _ = BeforeSuite(func() {
	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(2 * time.Second)

	cluster := kindClusterName()
	ns := testNamespace()
	nodeImage := os.Getenv("KIND_NODE_IMAGE")

	By(fmt.Sprintf("ensuring Kind cluster %q exists (node image: %s)", cluster, nodeImageOrDefault(nodeImage)))
	out, _ := runOrErr("kind", "get", "clusters")
	if !strings.Contains(out, cluster) {
		args := []string{"create", "cluster", "--name", cluster}
		if nodeImage != "" {
			args = append(args, "--image", nodeImage)
		}
		runMust("kind", args...)
	}

	By("setting kubectl context to Kind cluster")
	runMust("kubectl", "cluster-info", "--context", fmt.Sprintf("kind-%s", cluster))

	By("building operator Docker image")
	runFromDir(orgDir(), "docker", "build",
		"-f", "astradns-operator/Dockerfile",
		"-t", operatorImg, ".")

	By("building agent Docker image")
	runFromDir(orgDir(), "docker", "build",
		"-f", "astradns-agent/Dockerfile",
		"-t", agentImg, ".")

	By("loading images into Kind cluster")
	runMust("kind", "load", "docker-image", operatorImg, "--name", cluster)
	runMust("kind", "load", "docker-image", agentImg, "--name", cluster)

	By("creating test namespace")
	_, _ = runOrErr("kubectl", "create", "namespace", ns)

	By("installing Helm chart")
	runMust("helm", "upgrade", "--install", helmRelease, chartPath,
		"--namespace", ns,
		"--set", "operator.image.repository=astradns-operator",
		"--set", "operator.image.tag=integration",
		"--set", "agent.image.repository=astradns-agent",
		"--set", "agent.image.tag=integration",
		"--set", "crds.install=true",
		// Kind control-plane nodes are tainted; tolerate so pods schedule.
		"--set", "agent.tolerations[0].key=node-role.kubernetes.io/control-plane",
		"--set", "agent.tolerations[0].operator=Exists",
		"--set", "agent.tolerations[0].effect=NoSchedule",
		"--set", "operator.tolerations[0].key=node-role.kubernetes.io/control-plane",
		"--set", "operator.tolerations[0].operator=Exists",
		"--set", "operator.tolerations[0].effect=NoSchedule",
	)

	By("waiting for operator pod to be running")
	Eventually(func(g Gomega) {
		phase, err := runOrErr("kubectl", "get", "pods",
			"-n", ns,
			"-l", "app.kubernetes.io/component=operator",
			"-o", "jsonpath={.items[0].status.phase}")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(phase).To(Equal("Running"))
	}, 3*time.Minute, 2*time.Second).Should(Succeed())

	By("waiting for agent pod to be running")
	Eventually(func(g Gomega) {
		phase, err := runOrErr("kubectl", "get", "pods",
			"-n", ns,
			"-l", "app.kubernetes.io/component=agent",
			"-o", "jsonpath={.items[0].status.phase}")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(phase).To(Equal("Running"))
	}, 3*time.Minute, 2*time.Second).Should(Succeed())
})

var _ = AfterSuite(func() {
	cluster := kindClusterName()
	ns := testNamespace()

	By("uninstalling Helm release")
	_, _ = runOrErr("helm", "uninstall", helmRelease, "--namespace", ns)

	By("deleting test namespace")
	_, _ = runOrErr("kubectl", "delete", "namespace", ns, "--ignore-not-found")

	if os.Getenv("KEEP_KIND_CLUSTER") != "" {
		GinkgoWriter.Println("KEEP_KIND_CLUSTER set — skipping cluster deletion")
		return
	}

	By("deleting Kind cluster")
	_, _ = runOrErr("kind", "delete", "cluster", "--name", cluster)
})

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func projectDir() string {
	wd, err := os.Getwd()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func orgDir() string {
	return filepath.Clean(filepath.Join(projectDir(), ".."))
}

// runMust executes a command from the project root and fails the test on error.
func runMust(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = projectDir()
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	GinkgoWriter.Printf("+ %s %s\n", name, strings.Join(args, " "))
	output, err := cmd.CombinedOutput()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(),
		"%s %s failed:\n%s", name, strings.Join(args, " "), string(output))
	return strings.TrimSpace(string(output))
}

// runOrErr executes a command and returns output + error without failing.
func runOrErr(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = projectDir()
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	GinkgoWriter.Printf("+ %s %s\n", name, strings.Join(args, " "))
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

// runFromDir executes a command from a specific directory.
func runFromDir(dir, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	GinkgoWriter.Printf("+ [%s] %s %s\n", filepath.Base(dir), name, strings.Join(args, " "))
	output, err := cmd.CombinedOutput()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(),
		"%s %s failed in %s:\n%s", name, strings.Join(args, " "), dir, string(output))
	return strings.TrimSpace(string(output))
}

// applyYAML pipes the given YAML string into kubectl apply.
func applyYAML(yaml string) {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Dir = projectDir()
	cmd.Stdin = strings.NewReader(yaml)
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	GinkgoWriter.Printf("+ kubectl apply -f - (inline YAML)\n")
	output, err := cmd.CombinedOutput()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(),
		"kubectl apply failed:\n%s", string(output))
}

// nonEmpty splits output by newlines and returns non-blank entries.
func nonEmpty(output string) []string {
	var result []string
	for _, line := range strings.Split(output, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// configMapName returns the Helm-generated agent ConfigMap name.
func configMapName() string {
	return fmt.Sprintf("%s-agent-config", helmRelease)
}

func nodeImageOrDefault(img string) string {
	if img == "" {
		return "kind default"
	}
	return img
}
