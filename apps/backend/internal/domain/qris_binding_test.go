package domain

import "testing"

func TestValidateQrisPayloadBinding(t *testing.T) {
	t.Parallel()

	valid := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	uppercase := "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF"
	short := "0123456789abcdef"

	tests := []struct {
		name    string
		method  PaymentMethod
		hash    *string
		wantErr bool
	}{
		{name: "qris valid", method: PaymentMethodQRIS, hash: &valid},
		{name: "qris missing", method: PaymentMethodQRIS, wantErr: true},
		{name: "qris uppercase", method: PaymentMethodQRIS, hash: &uppercase, wantErr: true},
		{name: "qris short", method: PaymentMethodQRIS, hash: &short, wantErr: true},
		{name: "cash empty", method: PaymentMethodCash},
		{name: "cash rejects hash", method: PaymentMethodCash, hash: &valid, wantErr: true},
		{name: "legacy empty", method: PaymentMethodLegacy},
		{name: "legacy rejects hash", method: PaymentMethodLegacy, hash: &valid, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateQrisPayloadBinding(test.method, test.hash)
			if IsCode(err, CodeValidation) != test.wantErr {
				t.Fatalf("validation error=%v, wantErr=%v", err, test.wantErr)
			}
		})
	}
}
