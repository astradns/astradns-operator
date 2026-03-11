package controllers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	operatorconfig "github.com/astradns/astradns-operator/pkg/engineconfig"
	_ "github.com/astradns/astradns-operator/pkg/engineconfig/unbound"
	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	"github.com/astradns/astradns-types/engine"
	typesengineconfig "github.com/astradns/astradns-types/engineconfig"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	cfg         *rest.Config
	k8sClient   client.Client
	testEnv     *envtest.Environment
	managerStop context.CancelFunc
)

const (
	eventuallyTimeout = 30 * time.Second
	eventuallyPoll    = 250 * time.Millisecond
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	ctrl.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	binaryAssets := resolveBinaryAssetsDirectory()
	if binaryAssets == "" {
		Skip("envtest binaries not found; set KUBEBUILDER_ASSETS or run make setup-envtest")
	}

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		BinaryAssetsDirectory: binaryAssets,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	scheme := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		LeaderElection:         false,
	})
	Expect(err).NotTo(HaveOccurred())

	configRenderer, err := typesengineconfig.NewRenderer(engine.EngineUnbound)
	Expect(err).NotTo(HaveOccurred())

	configGen := &operatorconfig.DefaultConfigGenerator{}
	Expect((&DNSUpstreamPoolReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		ConfigGen:      configGen,
		ConfigRenderer: configRenderer,
		Recorder:       mgr.GetEventRecorder("dnsupstreampool-controller-test"),
	}).SetupWithManager(mgr)).To(Succeed())

	Expect((&DNSCacheProfileReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorder("dnscacheprofile-controller-test"),
	}).SetupWithManager(mgr)).To(Succeed())

	Expect((&ExternalDNSPolicyReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorder("externaldnspolicy-controller-test"),
	}).SetupWithManager(mgr)).To(Succeed())

	managerCtx, cancel := context.WithCancel(context.Background())
	managerStop = cancel

	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(managerCtx)).To(Succeed())
	}()

	Eventually(func() bool {
		return mgr.GetCache().WaitForCacheSync(managerCtx)
	}, eventuallyTimeout, eventuallyPoll).Should(BeTrue())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if managerStop != nil {
		managerStop()
	}
	if testEnv != nil {
		Expect(testEnv.Stop()).To(Succeed())
	}
})

func createNamespace(prefix string) string {
	namespace := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	obj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
	return namespace
}

func resolveBinaryAssetsDirectory() string {
	if fromEnv := os.Getenv("KUBEBUILDER_ASSETS"); fromEnv != "" {
		if absolute, ok := resolveCandidatePath(fromEnv); ok {
			return absolute
		}
		if absolute, ok := resolveCandidatePath(filepath.Join("..", fromEnv)); ok {
			return absolute
		}
	}

	candidates, err := filepath.Glob(filepath.Join("..", "bin", "k8s", "*"))
	if err != nil {
		return ""
	}
	for _, candidate := range candidates {
		if absolute, ok := resolveCandidatePath(candidate); ok {
			return absolute
		}
	}

	return ""
}

func resolveCandidatePath(path string) (string, bool) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}

	if !fileExists(filepath.Join(absolutePath, "etcd")) {
		return "", false
	}
	if !fileExists(filepath.Join(absolutePath, "kube-apiserver")) {
		return "", false
	}

	return absolutePath, true
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
