package domain

import "testing"

func TestValidatePaymentTotal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		method  PaymentMethod
		total   int64
		wantErr bool
	}{
		{
			name:   "qris below maximum",
			method: PaymentMethodQRIS,
			total:  MaxQRISTransactionTotal - 1,
		},
		{
			name:   "qris at maximum",
			method: PaymentMethodQRIS,
			total:  MaxQRISTransactionTotal,
		},
		{
			name:    "qris above maximum",
			method:  PaymentMethodQRIS,
			total:   MaxQRISTransactionTotal + 1,
			wantErr: true,
		},
		{
			name:   "cash above qris maximum",
			method: PaymentMethodCash,
			total:  MaxQRISTransactionTotal + 1,
		},
		{
			name:   "legacy above qris maximum",
			method: PaymentMethodLegacy,
			total:  MaxQRISTransactionTotal + 1,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := ValidatePaymentTotal(test.method, test.total)
			if test.wantErr {
				if !IsCode(err, CodeValidation) {
					t.Fatalf("expected validation error, got %v", err)
				}
				domainErr := AsError(err)
				if domainErr.Details["field"] != "total" ||
					domainErr.Details["maximum"] != MaxQRISTransactionTotal ||
					domainErr.Details["paymentMethod"] != PaymentMethodQRIS {
					t.Fatalf("unexpected validation details: %+v", domainErr.Details)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected total to be accepted, got %v", err)
			}
		})
	}
}
