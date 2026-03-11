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
					{Address: "1.1.1.1", Port: 53},
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
			g.Expect(configMap.Data).To(HaveKey("config.json"))
			g.Expect(configMap.Data["config.json"]).To(ContainSubstring(`"address": "1.1.1.1"`))
			g.Expect(configMap.Data["config.json"]).To(ContainSubstring(`"port": 5353`))
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
					{Address: "1.1.1.1", Port: 53},
					{Address: "8.8.8.8", Port: 53},
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())

		Eventually(func(g Gomega) {
			configMap := &corev1.ConfigMap{}
			err := k8sClient.Get(
				context.Background(),
				types.NamespacedName{Name: agentConfigMapName, Namespace: namespace},
				configMap,
			)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(configMap.Data["config.json"]).To(ContainSubstring(`"address": "8.8.8.8"`))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())

		Eventually(func() error {
			current := &v1alpha1.DNSUpstreamPool{}
			if err := k8sClient.Get(
				context.Background(),
				types.NamespacedName{Name: poolName, Namespace: namespace},
				current,
			); err != nil {
				return err
			}
			for _, upstream := range current.Spec.Upstreams {
				if upstream.Address == "9.9.9.9" {
					return nil
				}
			}
			current.Spec.Upstreams = append(current.Spec.Upstreams, v1alpha1.Upstream{Address: "9.9.9.9", Port: 53})
			return k8sClient.Update(context.Background(), current)
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())

		Eventually(func(g Gomega) {
			configMap := &corev1.ConfigMap{}
			err := k8sClient.Get(
				context.Background(),
				types.NamespacedName{Name: agentConfigMapName, Namespace: namespace},
				configMap,
			)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(configMap.Data).To(HaveKey("config.json"))
			g.Expect(configMap.Data["config.json"]).To(ContainSubstring(`"address": "9.9.9.9"`))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	It("removes rendered key when a pool is deleted", func() {
		namespace := createNamespace("pool-delete")
		poolName := "pool-delete"

		pool := &v1alpha1.DNSUpstreamPool{
			ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace},
			Spec: v1alpha1.DNSUpstreamPoolSpec{
				Upstreams: []v1alpha1.Upstream{{Address: "1.1.1.1", Port: 53}},
			},
		}
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())

		Eventually(func(g Gomega) {
			configMap := &corev1.ConfigMap{}
			err := k8sClient.Get(
				context.Background(),
				types.NamespacedName{Name: agentConfigMapName, Namespace: namespace},
				configMap,
			)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(configMap.Data).To(HaveKey("config.json"))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())

		Expect(k8sClient.Delete(context.Background(), pool)).To(Succeed())

		Eventually(func() bool {
			configMap := &corev1.ConfigMap{}
			err := k8sClient.Get(
				context.Background(),
				types.NamespacedName{Name: agentConfigMapName, Namespace: namespace},
				configMap,
			)
			if apierrors.IsNotFound(err) {
				return true
			}
			if err != nil {
				return false
			}
			_, exists := configMap.Data["config.json"]
			return !exists
		}, eventuallyTimeout, eventuallyPoll).Should(BeTrue())
	})

	It("rejects pools with no upstreams at CRD validation", func() {
		namespace := createNamespace("pool-invalid")
		poolName := "pool-invalid"

		pool := &v1alpha1.DNSUpstreamPool{
			ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace},
			Spec: v1alpha1.DNSUpstreamPoolSpec{
				Upstreams: []v1alpha1.Upstream{},
			},
		}
		Expect(k8sClient.Create(context.Background(), pool)).NotTo(Succeed())
	})

	It("uses the oldest pool as active when multiple pools exist", func() {
		namespace := createNamespace("pool-multi")

		poolZ := &v1alpha1.DNSUpstreamPool{
			ObjectMeta: metav1.ObjectMeta{Name: "zeta", Namespace: namespace},
			Spec: v1alpha1.DNSUpstreamPoolSpec{
				Upstreams: []v1alpha1.Upstream{{Address: "9.9.9.9", Port: 53}},
			},
		}
		Expect(k8sClient.Create(context.Background(), poolZ)).To(Succeed())

		poolA := &v1alpha1.DNSUpstreamPool{
			ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: namespace},
			Spec: v1alpha1.DNSUpstreamPoolSpec{
				Upstreams: []v1alpha1.Upstream{{Address: "1.1.1.1", Port: 53}},
			},
		}
		Expect(k8sClient.Create(context.Background(), poolA)).To(Succeed())

		Eventually(func(g Gomega) {
			configMap := &corev1.ConfigMap{}
			err := k8sClient.Get(
				context.Background(),
				types.NamespacedName{Name: agentConfigMapName, Namespace: namespace},
				configMap,
			)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(configMap.Data["config.json"]).To(ContainSubstring(`"address": "9.9.9.9"`))
			g.Expect(configMap.Data["config.json"]).NotTo(ContainSubstring(`"address": "1.1.1.1"`))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())

		Eventually(func(g Gomega) {
			current := &v1alpha1.DNSUpstreamPool{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: "alpha", Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, upstreamPoolReadyCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(condition.Reason).To(Equal(supersededPoolReason))
			g.Expect(condition.Message).To(ContainSubstring(`"zeta"`))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})
})
