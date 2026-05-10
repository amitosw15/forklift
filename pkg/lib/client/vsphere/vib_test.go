package vsphere

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestVSphere(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VSphere Client Suite")
}

var _ = Describe("ShouldSkipVIBCheck", func() {
	It("should not skip when lastTransitionTime is zero", func() {
		Expect(ShouldSkipVIBCheck(time.Time{}, 15*time.Minute)).To(BeFalse())
	})

	It("should skip when within cache duration", func() {
		recent := time.Now().Add(-5 * time.Minute)
		Expect(ShouldSkipVIBCheck(recent, 15*time.Minute)).To(BeTrue())
	})

	It("should not skip when cache duration has elapsed", func() {
		old := time.Now().Add(-20 * time.Minute)
		Expect(ShouldSkipVIBCheck(old, 15*time.Minute)).To(BeFalse())
	})

	It("should not skip when exactly at cache boundary", func() {
		boundary := time.Now().Add(-15 * time.Minute)
		Expect(ShouldSkipVIBCheck(boundary, 15*time.Minute)).To(BeFalse())
	})

	It("should handle zero cache duration", func() {
		recent := time.Now().Add(-1 * time.Second)
		Expect(ShouldSkipVIBCheck(recent, 0)).To(BeFalse())
	})
})
