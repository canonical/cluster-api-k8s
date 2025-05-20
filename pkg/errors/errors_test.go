package errors

import (
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestRequeueOnK8sdProxyError(t *testing.T) {
	notFoundErr := &K8sdProxyNotFound{NodeName: "foo"}
	notReadyErr := &K8sdProxyNotReady{PodName: "lish"}
	otherErr := fmt.Errorf("bar err")

	tests := []struct {
		name      string
		err       error
		expectErr bool
	}{
		{
			name: "k8sd-proxy not found",
			err:  notFoundErr,
		},
		{
			name: "k8sd-proxy not ready",
			err:  notFoundErr,
		},
		{
			name: "wrapped error",
			err:  fmt.Errorf("wrapped error: %w", fmt.Errorf("another wrap: %w", notFoundErr)),
		},
		{
			name: "joined error",
			err:  fmt.Errorf("wrapped joined error: %w", errors.Join(otherErr, notReadyErr)),
		},
		{
			name:      "other error",
			err:       otherErr,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			result, err := RequeueOnK8sdProxyError(tt.err)

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(result.IsZero()).To(BeTrue())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result.RequeueAfter).To(Equal(30 * time.Second))
			}
		})
	}
}
