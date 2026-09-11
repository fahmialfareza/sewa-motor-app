package domain

import (
	"fmt"
	"strings"
	"testing"
)

func staticQRISTestPayload(change func(string) string) string {
	tlv := func(tag, value string) string { return fmt.Sprintf("%s%02d%s", tag, len(value), value) }
	body := tlv("00", "01") + tlv("01", "11") + tlv("26", tlv("00", "ID.TEST.WWW")+tlv("01", "123456")) + tlv("52", "1234") + tlv("53", "360") + tlv("58", "ID") + tlv("59", "TEST MERCHANT") + tlv("60", "JAKARTA")
	if change != nil {
		body = change(body)
	}
	body += "6304"
	crc := uint16(0xffff)
	for _, b := range []byte(body) {
		crc ^= uint16(b) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return body + fmt.Sprintf("%04X", crc)
}

func TestStaticMerchantValidation(t *testing.T) {
	valid := staticQRISTestPayload(nil)
	if err := ValidateStaticQRIS(valid); err != nil {
		t.Fatal(err)
	}
	tests := map[string]string{
		"invalid crc":     valid[:len(valid)-1] + "Z",
		"dynamic":         staticQRISTestPayload(func(s string) string { return strings.Replace(s, "010211", "010212", 1) }),
		"embedded amount": staticQRISTestPayload(func(s string) string { return strings.Replace(s, "5802ID", "540410005802ID", 1) }),
		"signed MCC":      staticQRISTestPayload(func(s string) string { return strings.Replace(s, "52041234", "5204+123", 1) }),
		"wrong currency":  staticQRISTestPayload(func(s string) string { return strings.Replace(s, "5303360", "5303840", 1) }),
		"non-numeric tag": staticQRISTestPayload(func(s string) string { return strings.Replace(s, "010211", "+10211", 1) }),
		"duplicate tag":   staticQRISTestPayload(func(s string) string { return strings.Replace(s, "010211", "010211010211", 1) }),
	}
	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			if !IsCode(ValidateStaticQRIS(payload), CodeValidation) {
				t.Fatal("invalid merchant payload accepted")
			}
		})
	}
}
