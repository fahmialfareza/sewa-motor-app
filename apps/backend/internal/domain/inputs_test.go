package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestCheckedLineTotal(t *testing.T) {
	got, err := CheckedLineTotal(70_000, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got != 210_000 {
		t.Fatalf("got %d, want 210000", got)
	}
}

func TestValidateItemsAllowsSamePackageDifferentRevision(t *testing.T) {
	id := uuid.New()
	err := ValidateItems([]ItemInput{
		{PackageID: id, PackageRevision: 1, Quantity: 1},
		{PackageID: id, PackageRevision: 257, Quantity: 1},
	})
	if err != nil {
		t.Fatalf("different revisions should be distinct: %v", err)
	}
}

func TestValidateItemsRejectsDuplicateSnapshot(t *testing.T) {
	id := uuid.New()
	err := ValidateItems([]ItemInput{
		{PackageID: id, PackageRevision: 1, Quantity: 1},
		{PackageID: id, PackageRevision: 1, Quantity: 2},
	})
	if !IsCode(err, CodeValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestPasswordContractRequiresTwelveCharacters(t *testing.T) {
	if err := ValidatePassword("12345678901"); !IsCode(err, CodeValidation) {
		t.Fatalf("expected 11-character password to fail, got %v", err)
	}
	if err := ValidatePassword("123456789012"); err != nil {
		t.Fatalf("expected 12-character password to pass: %v", err)
	}
}

func TestCanCorrectTransactionUsesOwnerOrSuperadmin(t *testing.T) {
	ownerID := uuid.New()
	otherID := uuid.New()

	if !CanCorrectTransaction(RoleAdmin, ownerID, ownerID) {
		t.Fatal("admin should be able to correct their own transaction")
	}
	if CanCorrectTransaction(RoleAdmin, otherID, ownerID) {
		t.Fatal("admin should not be able to correct another user's transaction")
	}
	if !CanCorrectTransaction(RoleSuperadmin, otherID, ownerID) {
		t.Fatal("superadmin should be able to correct any transaction")
	}
}

func TestPaymentMethodsSeparateSelectableFromLegacy(t *testing.T) {
	t.Parallel()

	for _, method := range []PaymentMethod{PaymentMethodCash, PaymentMethodQRIS} {
		if err := ValidateSelectablePaymentMethod(method); err != nil {
			t.Fatalf("%s should be selectable: %v", method, err)
		}
	}
	if !PaymentMethodLegacy.Valid() {
		t.Fatal("legacy must remain readable")
	}
	if err := ValidateSelectablePaymentMethod(PaymentMethodLegacy); !IsCode(err, CodeValidation) {
		t.Fatalf("legacy must not be selectable, got %v", err)
	}
}

func TestPaymentOutcomeRejectsPending(t *testing.T) {
	t.Parallel()

	if err := ValidatePaymentOutcome(PaymentStatusSuccess); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePaymentOutcome(PaymentStatusFailed); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePaymentOutcome(PaymentStatusPending); !IsCode(err, CodeValidation) {
		t.Fatalf("pending must be server-owned, got %v", err)
	}
}
