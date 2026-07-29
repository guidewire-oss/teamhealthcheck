// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestV1Suite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "API V1 Suite")
}
