package vsphere

import (
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestVSphereHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VSphere Handler Suite")
}

var _ = Describe("sanitizeK8sName", func() {
	It("should lowercase the name", func() {
		Expect(sanitizeK8sName("MyHost")).To(Equal("myhost"))
	})

	It("should replace dots with dashes", func() {
		Expect(sanitizeK8sName("cat-02.redhat.com")).To(Equal("cat-02-redhat-com"))
	})

	It("should replace underscores with dashes", func() {
		Expect(sanitizeK8sName("my_esxi_host")).To(Equal("my-esxi-host"))
	})

	It("should strip non-alphanumeric characters", func() {
		Expect(sanitizeK8sName("host@#$%name")).To(Equal("hostname"))
	})

	It("should trim leading and trailing dashes", func() {
		Expect(sanitizeK8sName(".host.")).To(Equal("host"))
	})

	It("should return 'host' for empty result", func() {
		Expect(sanitizeK8sName("@#$")).To(Equal("host"))
	})

	It("should return 'host' for empty input", func() {
		Expect(sanitizeK8sName("")).To(Equal("host"))
	})

	It("should truncate to 253 characters", func() {
		long := strings.Repeat("a", 300)
		result := sanitizeK8sName(long)
		Expect(result).To(HaveLen(253))
	})

	It("should handle typical ESXi FQDN", func() {
		Expect(sanitizeK8sName("esxi-01.datacenter.example.com")).To(Equal("esxi-01-datacenter-example-com"))
	})

	It("should handle IP-style names", func() {
		Expect(sanitizeK8sName("10.0.0.1")).To(Equal("10-0-0-1"))
	})
})
