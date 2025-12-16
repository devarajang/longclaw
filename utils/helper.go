package utils

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	mathrand "math/rand"
	"os"
	"time"

	"github.com/devarajang/longclaw/dtos"
	"github.com/devarajang/longclaw/iso"
)

var GlobalIsoSpec *iso.IsoSpec

var CardsList []dtos.Card

func LoadCards(dataPath string) error {

	file, err := os.Open(dataPath + "test_cards.json")
	if err != nil {
		return err
	}

	decoder := json.NewDecoder(file)
	decoder.Decode(&CardsList)
	fmt.Println(CardsList)
	return nil
}

func GetRandomCard() dtos.Card {
	randNum := mathrand.Intn(len(CardsList))
	return CardsList[randNum]
}

func GenerateTimestampID() string {
	// Get current timestamp in milliseconds
	timestamp := time.Now().UnixNano() / 1e6

	// Generate random 3-digit suffix
	randNum, _ := rand.Int(rand.Reader, big.NewInt(900))
	randNum = randNum.Add(randNum, big.NewInt(100))

	// Combine timestamp (last 9 digits) with random suffix
	timestampStr := fmt.Sprintf("%09d", timestamp%1000000000)
	randomStr := randNum.String()

	return timestampStr + randomStr
}

func Substr(input string, start int, length int) string {
	asRunes := []rune(input)

	if start >= len(asRunes) {
		return ""
	}

	if start+length > len(asRunes) {
		length = len(asRunes) - start
	}

	return string(asRunes[start : start+length])
}
