package controllers

import (
	"context"

	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
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

	createPolicyWithPoolAndSelector := func(
		namespace,
		poolName,
		policyName string,
		selectorNamespaces []string,
		upstreamPoolRef,
		cacheProfileRef string,
	) {
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
				Selector:        v1alpha1.PolicySelector{Namespaces: selectorNamespaces},
				UpstreamPoolRef: v1alpha1.ResourceRef{Name: upstreamPoolRef},
				CacheProfileRef: v1alpha1.ResourceRef{Name: cacheProfileRef},
			},
		}
		Expect(k8sClient.Create(context.Background(), policy)).To(Succeed())
	}

	createPolicyWithPool := func(namespace, poolName, policyName, upstreamPoolRef, cacheProfileRef string) {
		createPolicyWithPoolAndSelector(
			namespace,
			poolName,
			policyName,
			[]string{"target-ns"},
			upstreamPoolRef,
			cacheProfileRef,
		)
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

	It("enforces policy by annotating selected namespaces", func() {
		policyNamespace := createNamespace("policy-enforce")
		targetNamespace := createNamespace("target-enforce")
		poolName := "pool-enforce"
		policyName := "policy-enforce"
		createDefaultProfile(policyNamespace)

		createPolicyWithPoolAndSelector(
			policyNamespace,
			poolName,
			policyName,
			[]string{targetNamespace},
			poolName,
			"default",
		)

		Eventually(func(g Gomega) {
			namespace := &corev1.Namespace{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: targetNamespace}, namespace)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(namespace.Annotations).To(HaveKeyWithValue(
				namespacePolicyNameAnnotation,
				policyNamespace+"/"+policyName,
			))
			g.Expect(namespace.Annotations).To(HaveKeyWithValue(namespacePolicyUpstreamRefAnnotation, poolName))
			g.Expect(namespace.Annotations).To(HaveKeyWithValue(namespacePolicyCacheRefAnnotation, "default"))
			g.Expect(namespace.Annotations).To(HaveKey(namespacePolicyOwnerUIDAnnotation))
			g.Expect(namespace.Annotations[namespacePolicyOwnerUIDAnnotation]).NotTo(BeEmpty())
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	It("sets Validated=False when another policy already manages a selected namespace", func() {
		targetNamespace := createNamespace("target-conflict")

		policyNamespaceA := createNamespace("policy-a")
		poolNameA := "pool-a"
		policyNameA := "policy-a"
		createDefaultProfile(policyNamespaceA)
		createPolicyWithPoolAndSelector(
			policyNamespaceA,
			poolNameA,
			policyNameA,
			[]string{targetNamespace},
			poolNameA,
			"default",
		)

		Eventually(func(g Gomega) {
			current := &v1alpha1.ExternalDNSPolicy{}
			err := k8sClient.Get(
				context.Background(),
				types.NamespacedName{Name: policyNameA, Namespace: policyNamespaceA},
				current,
			)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, externalPolicyValidatedCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())

		policyNamespaceB := createNamespace("policy-b")
		poolNameB := "pool-b"
		policyNameB := "policy-b"
		createDefaultProfile(policyNamespaceB)
		createPolicyWithPoolAndSelector(
			policyNamespaceB,
			poolNameB,
			policyNameB,
			[]string{targetNamespace},
			poolNameB,
			"default",
		)

		Eventually(func(g Gomega) {
			current := &v1alpha1.ExternalDNSPolicy{}
			err := k8sClient.Get(
				context.Background(),
				types.NamespacedName{Name: policyNameB, Namespace: policyNamespaceB},
				current,
			)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, externalPolicyValidatedCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(condition.Reason).To(Equal("EnforcementFailed"))
			g.Expect(condition.Message).To(ContainSubstring("already managed"))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	It("removes namespace policy annotations when policy is deleted", func() {
		policyNamespace := createNamespace("policy-cleanup")
		targetNamespace := createNamespace("target-cleanup")
		poolName := "pool-cleanup"
		policyName := "policy-cleanup"
		createDefaultProfile(policyNamespace)

		createPolicyWithPoolAndSelector(
			policyNamespace,
			poolName,
			policyName,
			[]string{targetNamespace},
			poolName,
			"default",
		)

		Eventually(func(g Gomega) {
			namespace := &corev1.Namespace{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: targetNamespace}, namespace)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(namespace.Annotations).To(HaveKey(namespacePolicyOwnerUIDAnnotation))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())

		policy := &v1alpha1.ExternalDNSPolicy{}
		Expect(k8sClient.Get(
			context.Background(),
			types.NamespacedName{Name: policyName, Namespace: policyNamespace},
			policy,
		)).To(Succeed())
		Expect(k8sClient.Delete(context.Background(), policy)).To(Succeed())

		Eventually(func(g Gomega) {
			namespace := &corev1.Namespace{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: targetNamespace}, namespace)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(namespace.Annotations).NotTo(HaveKey(namespacePolicyOwnerUIDAnnotation))
			g.Expect(namespace.Annotations).NotTo(HaveKey(namespacePolicyNameAnnotation))
			g.Expect(namespace.Annotations).NotTo(HaveKey(namespacePolicyUpstreamRefAnnotation))
			g.Expect(namespace.Annotations).NotTo(HaveKey(namespacePolicyCacheRefAnnotation))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	It("sets Validated=True when upstreamPoolRef uses dotted resource name", func() {
		namespace := createNamespace("policy-valid-dotted")
		poolName := "pool.v1"
		policyName := "policy-dotted"
		createDefaultProfile(namespace)

		createPolicyWithPool(namespace, poolName, policyName, poolName, "default")

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

		createPolicyWithPool(namespace, poolName, policyName, poolName, "nonexistent-profile")

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

	It("sets Validated=False when cacheProfileRef name is whitespace-only", func() {
		namespace := createNamespace("policy-cache-whitespace")
		poolName := "pool-cache-whitespace"
		policyName := "policy-cache-whitespace"

		createPolicyWithPool(namespace, poolName, policyName, poolName, "   ")

		Eventually(func(g Gomega) {
			current := &v1alpha1.ExternalDNSPolicy{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: policyName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, externalPolicyValidatedCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(condition.Message).To(ContainSubstring("spec.cacheProfileRef.name must not be whitespace"))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	It("sets Validated=False when upstreamPoolRef has leading or trailing whitespace", func() {
		namespace := createNamespace("policy-upstream-ref-padding")
		poolName := "pool-upstream-padding"
		policyName := "policy-upstream-padding"
		expectedMsg := "spec.upstreamPoolRef.name must not include leading or trailing whitespace"
		createDefaultProfile(namespace)

		createPolicyWithPool(namespace, poolName, policyName, poolName+" ", "default")

		Eventually(func(g Gomega) {
			current := &v1alpha1.ExternalDNSPolicy{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: policyName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, externalPolicyValidatedCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(condition.Message).To(ContainSubstring(expectedMsg))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	It("sets Validated=False when cacheProfileRef has leading or trailing whitespace", func() {
		namespace := createNamespace("policy-cache-ref-padding")
		poolName := "pool-cache-padding"
		policyName := "policy-cache-padding"
		expectedMsg := "spec.cacheProfileRef.name must not include leading or trailing whitespace"
		createDefaultProfile(namespace)

		createPolicyWithPool(namespace, poolName, policyName, poolName, "default ")

		Eventually(func(g Gomega) {
			current := &v1alpha1.ExternalDNSPolicy{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: policyName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, externalPolicyValidatedCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(condition.Message).To(ContainSubstring(expectedMsg))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	It("sets Validated=False when selector namespace entry is whitespace-only", func() {
		namespace := createNamespace("policy-invalid-selector-empty")
		poolName := "pool-selector-empty"
		policyName := "policy-selector-empty"
		createDefaultProfile(namespace)

		createPolicyWithPoolAndSelector(
			namespace,
			poolName,
			policyName,
			[]string{"target-ns", "   "},
			poolName,
			"default",
		)

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

	It("sets Validated=False when selector namespace entry has surrounding whitespace", func() {
		namespace := createNamespace("policy-invalid-selector-padding")
		poolName := "pool-selector-padding"
		policyName := "policy-selector-padding"
		createDefaultProfile(namespace)

		createPolicyWithPoolAndSelector(
			namespace,
			poolName,
			policyName,
			[]string{"target-ns", " target-ns"},
			poolName,
			"default",
		)

		Eventually(func(g Gomega) {
			current := &v1alpha1.ExternalDNSPolicy{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: policyName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, externalPolicyValidatedCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(condition.Message).To(
				ContainSubstring("spec.selector.namespaces[1] must not include leading or trailing whitespace"),
			)
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

	It("sets Validated=False when selector namespace list contains duplicates", func() {
		namespace := createNamespace("policy-selector-duplicates")
		poolName := "pool-selector-duplicates"
		policyName := "policy-selector-duplicates"
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
				Selector:        v1alpha1.PolicySelector{Namespaces: []string{"target-ns", "target-ns"}},
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
			g.Expect(condition.Message).To(ContainSubstring("spec.selector.namespaces[1]"))
			g.Expect(condition.Message).To(ContainSubstring("is duplicated"))
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
