package domain

// ReceiptIdentity is the immutable merchant profile selected at sale creation.
// It is independent of the provider-issued merchant name inside a QRIS payload.
type ReceiptIdentity struct {
	BusinessName string `json:"businessName"`
	Address      string `json:"address"`
	Phone        string `json:"phone"`
	Revision     int    `json:"revision"`
}
