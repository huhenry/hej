package microapp_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestMicroapp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Microapp Suite")
}
