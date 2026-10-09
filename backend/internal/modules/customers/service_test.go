package customers

import (
	"errors"
	"testing"

	"github.com/example/sistemaemgo/internal/modules/common"
)

func TestCustomerInputValidation(t *testing.T) {
	valid := []CustomerInput{
		{Name: "Maria Silva"},
		{Name: "Maria Silva", Email: ptr("maria@example.com")},
		{Name: "Maria Silva", Phone: ptr("(11) 99999-0000")},
		{Name: "Maria Silva", Email: ptr(""), Phone: ptr("")},
	}
	for _, input := range valid {
		if _, err := normalize(input); err != nil {
			t.Fatalf("valid input rejected: %+v err=%v", input, err)
		}
	}
	invalid := []CustomerInput{
		{Name: ""},
		{Name: "A"},
		{Name: "Nome\nInjetado"},
		{Name: "Maria Silva", Email: ptr("bad email")},
		{Name: "Maria Silva", Email: ptr("maria@example.com\nCc:steal@example.com")},
		{Name: "Maria Silva", Phone: ptr("123\n456")},
		{Name: "Maria Silva", Phone: ptr("1234567890123456789012345678901")},
	}
	for _, input := range invalid {
		if _, err := normalize(input); !errors.Is(err, common.ErrValidation) {
			t.Fatalf("invalid input accepted: %+v err=%v", input, err)
		}
	}
}

func ptr(value string) *string { return &value }
