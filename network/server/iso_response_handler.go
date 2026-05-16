package network

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/devarajang/longclaw/iso"
)

// Field index constants — public API uses (spec field number - 1).
const (
	fieldIdxAuthID       = 37  // spec field 38: Authorization ID Response
	fieldIdxResponseCode = 38  // spec field 39: Response Code
	fieldIdxCurrency     = 48  // spec field 49: Currency Code
	fieldIdxAddlAmounts  = 53  // spec field 54: Additional Amounts
	fieldIdxIssuerData   = 122 // spec field 123: Private/issuer response data
)

var requestMTISet = map[string]bool{
	"0100": true, "0101": true,
	"0200": true, "0201": true,
	"0420": true, "0421": true,
	"0800": true,
}

var responseMTISet = map[string]bool{
	"0110": true, "0111": true,
	"0210": true, "0211": true,
	"0430": true, "0431": true,
	"0810": true,
}

func isRequestMTI(mti string) bool  { return requestMTISet[mti] }
func isResponseMTI(mti string) bool { return responseMTISet[mti] }

func toResponseMTI(mti string) string {
	n, _ := strconv.Atoi(mti)
	return fmt.Sprintf("%04d", (n&0xFFFE)+10)
}

func buildResponse(req *iso.Iso8583Message) (*iso.Iso8583Message, error) {
	mti := req.GetMTI()
	resp := iso.NewResponseMessage(req, toResponseMTI(mti))

	// Network management: echo back with approved response code only.
	if mti == "0800" || mti == "0801" {
		resp.MarkField(fieldIdxResponseCode)
		resp.SetField(fieldIdxResponseCode, "00")
		return resp, nil
	}

	pan := req.GetField(1)    // spec field 2: PAN
	amount := req.GetField(3) // spec field 4: Amount
	// spec field 123 carries acquirer CVV/AVS tags (CV, AV, SUPPORT, RETURNBAL)
	field123 := req.GetField(fieldIdxIssuerData)

	responseCode := getResponseCode(pan, amount, field123)

	resp.MarkField(fieldIdxAuthID)
	resp.SetField(fieldIdxAuthID, time.Now().Format("150405"))

	resp.MarkField(fieldIdxResponseCode)
	resp.SetField(fieldIdxResponseCode, responseCode)

	resp.MarkField(fieldIdxIssuerData)
	resp.SetField(fieldIdxIssuerData, buildField123(getAVSResult(field123), getCVVResult(field123)))

	origAmt, _ := strconv.ParseInt(strings.TrimLeft(amount, "0"), 10, 64)
	currency := req.GetField(fieldIdxCurrency)
	if currency == "" {
		currency = "840"
	}

	if responseCode == "10" {
		resp.SetField(3, fmt.Sprintf("%012d", origAmt/2))
		resp.MarkField(fieldIdxAddlAmounts)
		resp.SetField(fieldIdxAddlAmounts, additionalAmount("00", "91", currency, origAmt))
	}

	if responseCode == "51" && strings.Contains(field123, "RETURNBAL") {
		resp.MarkField(fieldIdxAddlAmounts)
		resp.SetField(fieldIdxAddlAmounts, additionalAmount("00", "02", currency, int64(float64(origAmt)*0.8)))
	}

	return resp, nil
}

// getResponseCode derives the response code from the last digit of the PAN.
// Mirrors GetNetMessageResponseCode + GetResponseCodeWithPrimaryAccountNumber in the C# simulator.
func getResponseCode(pan, amount, field123 string) string {
	if len(pan) < 16 {
		return "96"
	}
	digit, err := strconv.Atoi(string(pan[15]))
	if err != nil {
		return "96"
	}
	amtInt, _ := strconv.ParseInt(strings.TrimLeft(amount, "0"), 10, 64)
	if amtInt == 0 && digit == 7 {
		digit = 0
	}
	return responseCodeFromDigit(digit, field123)
}

func responseCodeFromDigit(digit int, field123 string) string {
	// Digits 4 and 5 simulate slow issuer response.
	if digit == 4 || digit == 5 {
		time.Sleep(2 * time.Second)
	}
	switch digit {
	case 0, 1, 2, 3, 4:
		if strings.Contains(field123, "SUPPORT") {
			return "10" // partial approval
		}
		return "00"
	case 5:
		return "00"
	case 6:
		return []string{"05", "06", "07"}[rand.Intn(3)]
	case 7:
		return "51"
	case 8:
		return []string{"34", "33", "35"}[rand.Intn(3)]
	case 9:
		return []string{"SA", "SB", "SD", "SR", "T3", "T4", "T5"}[rand.Intn(7)]
	default:
		return "96"
	}
}

// getAVSResult reads the AV tag from field 123 and returns the AVS result code.
// Format in field 123: tag(2) + length(2) + value e.g. "AV2090210Main St"
func getAVSResult(field123 string) string {
	avData := extractTag(field123, "AV")
	if avData == "" || len(avData) < 9 {
		return "E"
	}
	zip := strings.TrimRight(avData[:9], " ")
	street := avData[9:]
	zip9 := len(zip) == 9 && strings.HasPrefix(zip, "9")
	zip5 := len(zip) >= 5 && (strings.HasPrefix(zip, "5") || strings.HasPrefix(zip, "3"))
	hasStreet := len(street) > 0 && street[0] >= '0' && street[0] <= '9'

	switch {
	case zip9 && hasStreet:
		return "X"
	case zip5 && hasStreet:
		return "Y"
	case zip5:
		return "Z"
	case zip9:
		return "W"
	case hasStreet:
		return "A"
	default:
		return "N"
	}
}

// getCVVResult reads the CV tag from field 123 and returns the CVV result code.
func getCVVResult(field123 string) string {
	cvVal := extractTag(field123, "CV")
	if cvVal == "" {
		return "N"
	}
	if len(cvVal) >= 3 && cvVal[len(cvVal)-3:] == "949" {
		return "M"
	}
	if cvVal == "838" {
		return "P"
	}
	return "N"
}

// extractTag finds tag (2 chars) + 2-char length + value within a field string.
func extractTag(field, tag string) string {
	ind := strings.Index(field, tag)
	if ind == -1 || len(field) < ind+4 {
		return ""
	}
	l, err := strconv.Atoi(field[ind+2 : ind+4])
	if err != nil || len(field) < ind+4+l {
		return ""
	}
	return field[ind+4 : ind+4+l]
}

// buildField123 constructs the issuer response portion of spec field 123.
// Format: TD + AR01<ar> + CR01<cr>
func buildField123(ar, cr string) string {
	return "TDAR01" + ar + "CR01" + cr
}

// additionalAmount builds one 20-char entry for ISO 8583 field 54.
// Format: accountType(2) + amountType(2) + currency(3) + "C" + amount(12)
func additionalAmount(accountType, amountType, currency string, amount int64) string {
	return fmt.Sprintf("%s%s%sC%012d", accountType, amountType, currency, amount)
}
