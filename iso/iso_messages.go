package iso

import (
	"fmt"
	"strconv"
	"strings"
)

type IsoMessage interface {
	GetMTI() string
	FormatPrint() string
	SetField(fieldNum int, value string)
	GetField(fieldNum int) string
}

type Iso8583Message struct {
	message         string
	primaryBitMap   *BitSet
	secondaryBitMap *BitSet
	fields          map[int]string
	isoSpec         *IsoSpec
}

// NewIso8583Message parses message using specRef. It returns an error, never panics,
// when the message is truncated or malformed.
func NewIso8583Message(message string, specRef *IsoSpec) (*Iso8583Message, error) {
	currIndex := 0
	isoMessage := Iso8583Message{message: message, isoSpec: specRef}

	// take returns the next n characters, or false if the message is too short
	take := func(n int) (string, bool) {
		if n < 0 || currIndex+n > len(message) {
			return "", false
		}
		value := message[currIndex : currIndex+n]
		currIndex += n
		return value, true
	}
	tooShort := func(what string) error {
		return fmt.Errorf("message too short for %s at offset %d (length %d)", what, currIndex, len(message))
	}

	if _, ok := take(4); !ok {
		return nil, tooShort("MTI")
	}
	primary, ok := take(16)
	if !ok {
		return nil, tooShort("primary bitmap")
	}
	isoMessage.primaryBitMap = new(BitSet)
	if err := isoMessage.primaryBitMap.SetString(primary, 16); err != nil {
		return nil, fmt.Errorf("invalid primary bitmap %q: %w", primary, err)
	}
	if isoMessage.primaryBitMap.GetBit(0) {
		secondary, ok := take(16)
		if !ok {
			return nil, tooShort("secondary bitmap")
		}
		isoMessage.secondaryBitMap = new(BitSet)
		if err := isoMessage.secondaryBitMap.SetString(secondary, 16); err != nil {
			return nil, fmt.Errorf("invalid secondary bitmap %q: %w", secondary, err)
		}
	}
	isoMessage.fields = make(map[int]string)

	parseMessage := func(ind int) (string, error) {
		fieldNo := ind + 1
		fieldDef, ok := specRef.FieldDefMap[fieldNo]
		if !ok {
			return "", fmt.Errorf("field %d is not in the spec", fieldNo)
		}

		lengthDigits := 0
		switch fieldDef.FieldType {
		case "Fixed":
			value, ok := take(fieldDef.MinLength)
			if !ok {
				return "", tooShort(fmt.Sprintf("field %d", fieldNo))
			}
			return value, nil
		case "LLVar":
			lengthDigits = 2
		case "LLLVar":
			lengthDigits = 3
		default:
			return "", fmt.Errorf("field %d has unknown type %q", fieldNo, fieldDef.FieldType)
		}

		lengthStr, ok := take(lengthDigits)
		if !ok {
			return "", tooShort(fmt.Sprintf("field %d length", fieldNo))
		}
		length, err := strconv.Atoi(lengthStr)
		if err != nil {
			return "", fmt.Errorf("field %d has invalid length %q", fieldNo, lengthStr)
		}
		value, ok := take(length)
		if !ok {
			return "", tooShort(fmt.Sprintf("field %d", fieldNo))
		}
		return value, nil
	}

	for ind := 1; ind < isoMessage.primaryBitMap.BitLen(); ind++ {
		if isoMessage.primaryBitMap.GetBit(ind) {
			retVal, err := parseMessage(ind)
			if err != nil {
				return nil, err
			}
			isoMessage.fields[ind] = retVal
		}
	}

	if isoMessage.secondaryBitMap != nil {
		for ind := 0; ind < isoMessage.secondaryBitMap.BitLen(); ind++ {
			if isoMessage.secondaryBitMap.GetBit(ind) {
				retVal, err := parseMessage(64 + ind)
				if err != nil {
					return nil, err
				}
				isoMessage.fields[64+ind] = retVal
			}
		}
	}

	return &isoMessage, nil
}

func (m *Iso8583Message) GetMTI() string {
	return m.message[0:4]
}

func (m *Iso8583Message) SetField(fieldNum int, value string) {
	m.fields[fieldNum] = value
}

func (m *Iso8583Message) GetField(fieldNum int) string {
	return m.fields[fieldNum]
}
func (m *Iso8583Message) FormatPrint() string {
	//fmt.Println(m.message)
	fieldCnt := 64
	var sb strings.Builder
	sb.WriteString("MTI : " + m.message[0:4])

	sb.WriteString("\nPrimaryBitMap : " + m.primaryBitMap.ToHex())

	if m.primaryBitMap.GetBit(0) {
		fieldCnt += 64
		sb.WriteString("\nSecondaryBitMap : " + m.secondaryBitMap.ToHex())
	}

	for field := range fieldCnt {
		val, ok := m.fields[field]
		if ok {
			sb.WriteString(fmt.Sprintf("\nDE[%v] : %v", field+1, val))
		}
	}
	return sb.String()
}

// NewResponseMessage deep-copies req and replaces the MTI with responseMTI.
// Call MarkField + SetField to add/override response-specific fields before FormatIso.
func NewResponseMessage(req *Iso8583Message, responseMTI string) *Iso8583Message {
	resp := &Iso8583Message{
		message:       responseMTI + req.message[4:],
		isoSpec:       req.isoSpec,
		primaryBitMap: &BitSet{data: req.primaryBitMap.data},
	}
	if req.secondaryBitMap != nil {
		resp.secondaryBitMap = &BitSet{data: req.secondaryBitMap.data}
	}
	resp.fields = make(map[int]string)
	for k, v := range req.fields {
		resp.fields[k] = v
	}
	return resp
}

// MarkField sets the bitmap bit for fieldIdx so FormatIso includes that field in output.
// fieldIdx is the same key used in GetField/SetField (spec field number − 1).
func (m *Iso8583Message) MarkField(fieldIdx int) {
	if fieldIdx < 64 {
		m.primaryBitMap.SetBit(fieldIdx, true)
	} else {
		m.primaryBitMap.SetBit(0, true) // enable secondary bitmap
		if m.secondaryBitMap == nil {
			m.secondaryBitMap = new(BitSet)
		}
		m.secondaryBitMap.SetBit(fieldIdx-64, true)
	}
}

func (m *Iso8583Message) FormatIso() string {
	//fmt.Println(m.message)
	fieldCnt := 64
	var sb strings.Builder
	sb.WriteString(m.GetMTI())

	sb.WriteString(m.primaryBitMap.ToHex())

	if m.primaryBitMap.GetBit(0) {
		fieldCnt += 64
		sb.WriteString(m.secondaryBitMap.ToHex())
	}

	for field := 1; field < fieldCnt; field++ {
		val, ok := m.fields[field]
		if ok {
			fieldSpec, ok := m.isoSpec.FieldDefMap[field+1]
			//fmt.Println(fieldSpec)
			if ok {
				switch fieldSpec.FieldType {
				case "Fixed":
					sb.WriteString(val)
				case "LLVar":
					//fmt.Println(len(val), val)
					sb.WriteString(fmt.Sprintf("%02d%v", len(val), val))
				case "LLLVar":
					//fmt.Println(len(val), val)
					sb.WriteString(fmt.Sprintf("%03d%v", len(val), val))
				}
			}
			//sb.WriteString(fmt.Sprintf("\nDE[%v] : %v", field, val))

		}
	}
	return sb.String()
}
