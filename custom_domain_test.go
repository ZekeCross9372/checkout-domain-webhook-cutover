package main

import "testing"

func TestTransitionCreatesPendingFulfillmentReceipt(t *testing.T) {
	tests := []struct {
		name string
		in   OnboardingRequest
		want string
	}{
		{name: "new checkout waits for verification", in: OnboardingRequest{OrderID: "o-42", CustomerID: "c-7", Domain: "shop.example", RecordName: "_verify", RecordValue: "proof"}, want: "awaiting_domain_verification"},
		{name: "receipt follows order", in: OnboardingRequest{OrderID: "o-99", CustomerID: "c-8", Domain: "store.example", RecordName: "_verify", RecordValue: "proof"}, want: "receipt-o-99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := transition(tt.in, "zone-1")
			if got.Order.Fulfillment != tt.want && got.Order.ReceiptID != tt.want {
				t.Fatalf("got fulfillment=%q receipt=%q, want %q", got.Order.Fulfillment, got.Order.ReceiptID, tt.want)
			}
		})
	}
}
