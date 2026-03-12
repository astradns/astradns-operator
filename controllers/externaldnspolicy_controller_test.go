package controllers

import (
	"context"

	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("ExternalDNSPolicy Controller", func() {
	createDefaultProfile := func(namespace string) {
		profile := &v1alpha1.DNSCacheProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: namespace},
		}
		Expect(k8sClient.Create(context.Background(), profile)).To(Succeed())
	}

	It("sets Validated=True when upstream pool exists", func() {
		namespace := createNamespace("policy-valid")
		poolName := "pool-a"
		policyName := "policy-a"
		createDefaultProfile(namespace)

		pool := &v1alpha1.DNSUpstreamPool{
			ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace},
			Spec: v1alpha1.DNSUpstreamPoolSpec{
				Upstreams: []v1alpha1.Upstream{{Address: "1.1.1.1", Port: 53}},
			},
		}
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())

		policy := &v1alpha1.ExternalDNSPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: namespace},
			Spec: v1alpha1.ExternalDNSPolicySpec{
				Selector:        v1alpha1.PolicySelector{Namespaces: []string{"target-ns"}},
				UpstreamPoolRef: v1alpha1.ResourceRef{Name: poolName},
				CacheProfileRef: v1alpha1.ResourceRef{Name: "default"},
			},
		}
		Expect(k8sClient.Create(context.Background(), policy)).To(Succeed())

		Eventually(func(g Gomega) {
			current := &v1alpha1.ExternalDNSPolicy{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: policyName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, externalPolicyValidatedCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	It("sets Validated=False when upstream pool is missing", func() {
		namespace := createNamespace("policy-missing")
		policyName := "policy-missing"
		createDefaultProfile(namespace)

		policy := &v1alpha1.ExternalDNSPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: namespace},
			Spec: v1alpha1.ExternalDNSPolicySpec{
				Selector:        v1alpha1.PolicySelector{Namespaces: []string{"target-ns"}},
				UpstreamPoolRef: v1alpha1.ResourceRef{Name: "does-not-exist"},
				CacheProfileRef: v1alpha1.ResourceRef{Name: "default"},
			},
		}
		Expect(k8sClient.Create(context.Background(), policy)).To(Succeed())

		Eventually(func(g Gomega) {
			current := &v1alpha1.ExternalDNSPolicy{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: policyName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, externalPolicyValidatedCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	It("sets Validated=False after referenced pool is deleted", func() {
		namespace := createNamespace("policy-delete")
		poolName := "pool-delete"
		policyName := "policy-delete"
		createDefaultProfile(namespace)

		pool := &v1alpha1.DNSUpstreamPool{
			ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace},
			Spec: v1alpha1.DNSUpstreamPoolSpec{
				Upstreams: []v1alpha1.Upstream{{Address: "1.1.1.1", Port: 53}},
			},
		}
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())

		policy := &v1alpha1.ExternalDNSPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: namespace},
			Spec: v1alpha1.ExternalDNSPolicySpec{
				Selector:        v1alpha1.PolicySelector{Namespaces: []string{"target-ns"}},
				UpstreamPoolRef: v1alpha1.ResourceRef{Name: poolName},
				CacheProfileRef: v1alpha1.ResourceRef{Name: "default"},
			},
		}
		Expect(k8sClient.Create(context.Background(), policy)).To(Succeed())

		Eventually(func(g Gomega) {
			current := &v1alpha1.ExternalDNSPolicy{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: policyName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, externalPolicyValidatedCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())

		Expect(k8sClient.Delete(context.Background(), pool)).To(Succeed())

		Eventually(func(g Gomega) {
			current := &v1alpha1.ExternalDNSPolicy{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: policyName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, externalPolicyValidatedCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	// --- Gap 7: ExternalDNSPolicy with missing cache profile ref ---

	It("sets Validated=False when cacheProfileRef points to nonexistent profile", func() {
		namespace := createNamespace("policy-no-cache")
		poolName := "pool-cache-missing"
		policyName := "policy-cache-missing"

		pool := &v1alpha1.DNSUpstreamPool{
			ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace},
			Spec: v1alpha1.DNSUpstreamPoolSpec{
				Upstreams: []v1alpha1.Upstream{{Address: "1.1.1.1", Port: 53}},
			},
		}
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())

		policy := &v1alpha1.ExternalDNSPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: namespace},
			Spec: v1alpha1.ExternalDNSPolicySpec{
				Selector:        v1alpha1.PolicySelector{Namespaces: []string{"target-ns"}},
				UpstreamPoolRef: v1alpha1.ResourceRef{Name: poolName},
				CacheProfileRef: v1alpha1.ResourceRef{Name: "nonexistent-profile"},
			},
		}
		Expect(k8sClient.Create(context.Background(), policy)).To(Succeed())

		Eventually(func(g Gomega) {
			current := &v1alpha1.ExternalDNSPolicy{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: policyName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, externalPolicyValidatedCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(condition.Message).To(ContainSubstring("nonexistent-profile"))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	It("sets Validated=False when selector namespace entry is whitespace-only", func() {
		namespace := createNamespace("policy-invalid-selector-empty")
		poolName := "pool-selector-empty"
		policyName := "policy-selector-empty"
		createDefaultProfile(namespace)

		pool := &v1alpha1.DNSUpstreamPool{
			ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace},
			Spec: v1alpha1.DNSUpstreamPoolSpec{
				Upstreams: []v1alpha1.Upstream{{Address: "1.1.1.1", Port: 53}},
			},
		}
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())

		policy := &v1alpha1.ExternalDNSPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: namespace},
			Spec: v1alpha1.ExternalDNSPolicySpec{
				Selector:        v1alpha1.PolicySelector{Namespaces: []string{"target-ns", "   "}},
				UpstreamPoolRef: v1alpha1.ResourceRef{Name: poolName},
				CacheProfileRef: v1alpha1.ResourceRef{Name: "default"},
			},
		}
		Expect(k8sClient.Create(context.Background(), policy)).To(Succeed())

		Eventually(func(g Gomega) {
			current := &v1alpha1.ExternalDNSPolicy{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: policyName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, externalPolicyValidatedCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(condition.Message).To(ContainSubstring("spec.selector.namespaces[1] must not be empty"))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	It("sets Validated=False when selector namespace entry is not DNS1123 label", func() {
		namespace := createNamespace("policy-invalid-selector-format")
		poolName := "pool-selector-format"
		policyName := "policy-selector-format"
		createDefaultProfile(namespace)

		pool := &v1alpha1.DNSUpstreamPool{
			ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace},
			Spec: v1alpha1.DNSUpstreamPoolSpec{
				Upstreams: []v1alpha1.Upstream{{Address: "1.1.1.1", Port: 53}},
			},
		}
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())

		policy := &v1alpha1.ExternalDNSPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: namespace},
			Spec: v1alpha1.ExternalDNSPolicySpec{
				Selector:        v1alpha1.PolicySelector{Namespaces: []string{"Prod-NS"}},
				UpstreamPoolRef: v1alpha1.ResourceRef{Name: poolName},
				CacheProfileRef: v1alpha1.ResourceRef{Name: "default"},
			},
		}
		Expect(k8sClient.Create(context.Background(), policy)).To(Succeed())

		Eventually(func(g Gomega) {
			current := &v1alpha1.ExternalDNSPolicy{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: policyName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, externalPolicyValidatedCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(condition.Message).To(ContainSubstring("spec.selector.namespaces[0]"))
			g.Expect(condition.Message).To(ContainSubstring("not a valid namespace name"))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	It("sets Validated=False when upstreamPoolRef name is whitespace-only", func() {
		namespace := createNamespace("policy-empty-pool")
		policyName := "policy-empty-pool"
		createDefaultProfile(namespace)

		policy := &v1alpha1.ExternalDNSPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: namespace},
			Spec: v1alpha1.ExternalDNSPolicySpec{
				Selector:        v1alpha1.PolicySelector{Namespaces: []string{"target-ns"}},
				UpstreamPoolRef: v1alpha1.ResourceRef{Name: " "},
				CacheProfileRef: v1alpha1.ResourceRef{Name: "default"},
			},
		}
		Expect(k8sClient.Create(context.Background(), policy)).To(Succeed())

		Eventually(func(g Gomega) {
			current := &v1alpha1.ExternalDNSPolicy{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: policyName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, externalPolicyValidatedCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(condition.Message).To(ContainSubstring("upstreamPoolRef.name is required"))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})
})
