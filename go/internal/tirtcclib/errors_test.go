package tirtcclib

import (
	"errors"
	"testing"
)

func TestErrorFromSendResult(t *testing.T) {
	for _, result := range []int{0, 1, 652} {
		if err := errorFromSendResult(result); err != nil {
			t.Errorf("errorFromSendResult(%d) = %v, want nil", result, err)
		}
	}
	if err := errorFromSendResult(-40006); !errors.Is(err, ErrBusy) {
		t.Fatalf("errorFromSendResult(-40006) = %v, want ErrBusy", err)
	}
}
