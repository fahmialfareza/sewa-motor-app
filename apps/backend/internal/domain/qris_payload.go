package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// ValidateStaticQRIS mirrors the mobile's MPM static payload validation. Only
// provider-issued ASCII payloads with a valid CRC may become tenant settings.
func ValidateStaticQRIS(payload string) error {
	invalid := func() error { return Validation("Gambar harus berisi QRIS merchant statis yang valid", nil) }
	if len(payload) < 12 || len(payload) > 2048 {
		return invalid()
	}
	for _, c := range []byte(payload) {
		if c < 32 || c > 126 {
			return invalid()
		}
	}
	values, err := qrisElements(payload)
	if err != nil {
		return invalid()
	}
	if values["00"] != "01" || values["01"] != "11" || values["53"] != "360" || values["58"] != "ID" || strings.TrimSpace(values["59"]) == "" || strings.TrimSpace(values["60"]) == "" {
		return invalid()
	}
	if len(values["52"]) != 4 || !qrisDigits(values["52"]) {
		return invalid()
	}
	for _, tag := range []string{"54", "55", "56", "57"} {
		if _, exists := values[tag]; exists {
			return invalid()
		}
	}
	merchant := false
	for i := 26; i <= 51; i++ {
		if nested, ok := values[fmt.Sprintf("%02d", i)]; ok {
			fields, e := qrisElements(nested)
			if e != nil {
				return invalid()
			}
			if fields["00"] != "" {
				merchant = true
			}
		}
	}
	if !merchant || len(values["63"]) != 4 || payload[len(payload)-8:len(payload)-4] != "6304" {
		return invalid()
	}
	crc := uint16(0xffff)
	for _, b := range []byte(payload[:len(payload)-4]) {
		crc ^= uint16(b) << 8
		for bit := 0; bit < 8; bit++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	if !strings.EqualFold(values["63"], fmt.Sprintf("%04X", crc)) {
		return invalid()
	}
	return nil
}

func qrisElements(payload string) (map[string]string, error) {
	result := map[string]string{}
	previous := -1
	for pos := 0; pos < len(payload); {
		if pos+4 > len(payload) {
			return nil, fmt.Errorf("truncated tag")
		}
		if !qrisDigits(payload[pos : pos+4]) {
			return nil, fmt.Errorf("non-numeric tag or length")
		}
		tag := payload[pos : pos+2]
		n, e := strconv.Atoi(tag)
		length, le := strconv.Atoi(payload[pos+2 : pos+4])
		pos += 4
		if e != nil || le != nil || n <= previous || length < 0 || pos+length > len(payload) {
			return nil, fmt.Errorf("invalid tag")
		}
		previous = n
		result[tag] = payload[pos : pos+length]
		pos += length
	}
	return result, nil
}

func qrisDigits(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
