package controllers

import (
	"context"

	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("DNSUpstreamPool Controller", func() {
	It("creates a ConfigMap when a pool with two upstreams is created", func() {
		namespace := createNamespace("pool-create")
		poolName := "pool-create"

		pool := &v1alpha1.DNSUpstreamPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      poolName,
				Namespace: namespace,
			},
			Spec: v1alpha1.DNSUpstreamPoolSpec{
				Upstreams: []v1alpha1.Upstream{
					{Address: "1.1.1.1"},
					{Address: "8.8.8.8", Port: 5353},
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())

		Eventually(func(g Gomega) {
			configMap := &corev1.ConfigMap{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      agentConfigMapName,
				Namespace: namespace,
			}, configMap)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(configMap.Data).To(HaveKey("unbound.conf"))
			g.Expect(configMap.Data["unbound.conf"]).To(ContainSubstring("forward-addr: 1.1.1.1"))
			g.Expect(configMap.Data["unbound.conf"]).To(ContainSubstring("forward-addr: 8.8.8.8@5353"))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())

		Eventually(func(g Gomega) {
			current := &v1alpha1.DNSUpstreamPool{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: poolName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, upstreamPoolReadyCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	It("updates ConfigMap when pool upstreams change", func() {
		namespace := createNamespace("pool-update")
		poolName := "pool-update"

		pool := &v1alpha1.DNSUpstreamPool{
			ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace},
			Spec: v1alpha1.DNSUpstreamPoolSpec{
				Upstreams: []v1alpha1.Upstream{
					{Address: "1.1.1.1"},
					{Address: "8.8.8.8"},
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())

		Eventually(func(g Gomega) {
			configMap := &corev1.ConfigMap{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: agentConfigMapName, Namespace: namespace}, configMap)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(configMap.Data["unbound.conf"]).To(ContainSubstring("forward-addr: 8.8.8.8"))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())

		Eventually(func() error {
			current := &v1alpha1.DNSUpstreamPool{}
			if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: poolName, Namespace: namespace}, current); err != nil {
				return err
			}
			for _, upstream := range current.Spec.Upstreams {
				if upstream.Address == "9.9.9.9" {
					return nil
				}
			}
			current.Spec.Upstreams = append(current.Spec.Upstreams, v1alpha1.Upstream{Address: "9.9.9.9"})
			return k8sClient.Update(context.Background(), current)
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())

		Eventually(func(g Gomega) {
			configMap := &corev1.ConfigMap{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: agentConfigMapName, Namespace: namespace}, configMap)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(configMap.Data).To(HaveKey("unbound.conf"))
			g.Expect(configMap.Data["unbound.conf"]).To(ContainSubstring("forward-addr: 9.9.9.9"))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	It("removes rendered key when a pool is deleted", func() {
		namespace := createNamespace("pool-delete")
		poolName := "pool-delete"

		pool := &v1alpha1.DNSUpstreamPool{
			ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace},
			Spec: v1alpha1.DNSUpstreamPoolSpec{
				Upstreams: []v1alpha1.Upstream{{Address: "1.1.1.1"}},
			},
		}
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())

		Eventually(func(g Gomega) {
			configMap := &corev1.ConfigMap{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: agentConfigMapName, Namespace: namespace}, configMap)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(configMap.Data).To(HaveKey("unbound.conf"))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())

		Expect(k8sClient.Delete(context.Background(), pool)).To(Succeed())

		Eventually(func() bool {
			configMap := &corev1.ConfigMap{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: agentConfigMapName, Namespace: namespace}, configMap)
			if apierrors.IsNotFound(err) {
				return true
			}
			if err != nil {
				return false
			}
			_, exists := configMap.Data["unbound.conf"]
			return !exists
		}, eventuallyTimeout, eventuallyPoll).Should(BeTrue())
	})

	It("sets Ready=False when the pool has no upstreams", func() {
		namespace := createNamespace("pool-invalid")
		poolName := "pool-invalid"

		pool := &v1alpha1.DNSUpstreamPool{
			ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace},
			Spec: v1alpha1.DNSUpstreamPoolSpec{
				Upstreams: []v1alpha1.Upstream{},
			},
		}
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())

		Eventually(func(g Gomega) {
			current := &v1alpha1.DNSUpstreamPool{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: poolName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, upstreamPoolReadyCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})
})
