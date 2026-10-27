package utils

import (
	"encoding/json"
	"fmt"
	"log"
	mathrand "math/rand"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/devarajang/longclaw/dtos"
	"github.com/devarajang/longclaw/iso"
)

var GlobalIsoSpec *iso.IsoSpec

var CardsList []dtos.Card

func LoadCards(dataPath string) error {
	// Support both file path and directory path for backward compatibility
	filePath := dataPath
	if !strings.HasSuffix(dataPath, ".json") {
		filePath = dataPath + "test_cards.json"
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&CardsList); err != nil {
		return err
	}
	if len(CardsList) == 0 {
		return fmt.Errorf("no cards found in %s", filePath)
	}
	// Log only the count: the card list holds PANs and CVVs
	log.Printf("Loaded %d test cards", len(CardsList))
	return nil
}

func GetRandomCard() dtos.Card {
	randNum := mathrand.Intn(len(CardsList))
	return CardsList[randNum]
}

// referenceCounter backs GenerateTimestampID. It is seeded from the clock at 100 IDs per millisecond,
// so references never repeat within a run and do not repeat after a restart unless the previous run
// averaged more than 100k IDs per second. The 12 digits wrap about every 115 days.
var referenceCounter atomic.Int64

func init() {
	referenceCounter.Store(time.Now().UnixMilli() * 100)
}

// GenerateTimestampID returns a unique 12-digit reference (field 37, Retrieval Reference Number).
// A random suffix on the millisecond collided under load: 0.5% of IDs at 10 per millisecond.
func GenerateTimestampID() string {
	return fmt.Sprintf("%012d", referenceCounter.Add(1)%1_000_000_000_000)
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
