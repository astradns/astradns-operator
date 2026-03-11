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

var _ = Describe("DNSCacheProfile Controller", func() {
	It("sets Active=True for a valid profile", func() {
		namespace := createNamespace("cache-valid")
		profileName := "default"

		profile := &v1alpha1.DNSCacheProfile{
			ObjectMeta: metav1.ObjectMeta{Name: profileName, Namespace: namespace},
			Spec: v1alpha1.DNSCacheProfileSpec{
				MaxEntries: 5000,
				PositiveTtl: v1alpha1.TtlConfig{
					MinSeconds: 30,
					MaxSeconds: 300,
				},
				NegativeTtl: v1alpha1.NegTtlConfig{Seconds: 30},
			},
		}
		Expect(k8sClient.Create(context.Background(), profile)).To(Succeed())

		Eventually(func(g Gomega) {
			current := &v1alpha1.DNSCacheProfile{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: profileName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, cacheProfileActiveCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})

	It("sets Active=False when TTL range is invalid", func() {
		namespace := createNamespace("cache-invalid")
		profileName := "invalid-ttl"

		profile := &v1alpha1.DNSCacheProfile{
			ObjectMeta: metav1.ObjectMeta{Name: profileName, Namespace: namespace},
			Spec: v1alpha1.DNSCacheProfileSpec{
				PositiveTtl: v1alpha1.TtlConfig{
					MinSeconds: 400,
					MaxSeconds: 300,
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), profile)).To(Succeed())

		Eventually(func(g Gomega) {
			current := &v1alpha1.DNSCacheProfile{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: profileName, Namespace: namespace}, current)
			g.Expect(err).NotTo(HaveOccurred())
			condition := meta.FindStatusCondition(current.Status.Conditions, cacheProfileActiveCondition)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		}, eventuallyTimeout, eventuallyPoll).Should(Succeed())
	})
})
