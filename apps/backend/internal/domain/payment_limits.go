package domain

const MaxQRISTransactionTotal int64 = 10_000_000

// ValidatePaymentTotal enforces payment-method-specific limits after the
// transaction total has been calculated from authoritative package prices.
func ValidatePaymentTotal(method PaymentMethod, total int64) error {
	if method != PaymentMethodQRIS || total <= MaxQRISTransactionTotal {
		return nil
	}
	return Validation(
		"Total transaksi QRIS maksimal Rp10.000.000",
		map[string]any{
			"field":         "total",
			"maximum":       MaxQRISTransactionTotal,
			"paymentMethod": PaymentMethodQRIS,
		},
	)
}
