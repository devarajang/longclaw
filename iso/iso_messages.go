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
	GetField(fieldNum int)
}

type Iso8583Message struct {
	message         string
	primaryBitMap   *BitSet
	secondaryBitMap *BitSet
	fields          map[int]string
	isoSpec         *IsoSpec
}

func NewIso8583Message(message string, specRef *IsoSpec) (*Iso8583Message, error) {
	currIndex := 4
	isoMessage := Iso8583Message{message: message, isoSpec: specRef}
	//fmt.Println(message)
	isoMessage.primaryBitMap = new(BitSet)
	isoMessage.primaryBitMap.SetString(isoMessage.message[currIndex:(currIndex+16)], 16)
	currIndex += 16
	if isoMessage.primaryBitMap.GetBit(0) {
		isoMessage.secondaryBitMap = new(BitSet)
		isoMessage.secondaryBitMap.SetString(isoMessage.message[currIndex:(currIndex+16)], 16)
		currIndex += 16
	}
	isoMessage.fields = make(map[int]string)

	parseMessage := func(ind int) (string, error) {
		value := ""
		fieldDef := specRef.FieldDefMap[ind+1]

		//fmt.Println(fieldDef)
		switch fieldDef.FieldType {
		case "Fixed":
			value = isoMessage.message[currIndex:(currIndex + fieldDef.MinLength)]
			currIndex += fieldDef.MinLength
		case "LLVar":
			lengthStr := isoMessage.message[currIndex:(currIndex + 2)]
			length, err := strconv.Atoi(lengthStr)
			if err != nil {
				return "", err
			}
			currIndex += 2
			value = isoMessage.message[currIndex:(currIndex + length)]
			currIndex += length
		case "LLLVar":
			lengthStr := isoMessage.message[currIndex:(currIndex + 3)]
			length, err := strconv.Atoi(lengthStr)
			if err != nil {
				return "", err
			}
			currIndex += 3
			value = isoMessage.message[currIndex:(currIndex + length)]
			currIndex += length
		}
		//fmt.Println(value)
		return value, nil
	}

	for ind := 1; ind < isoMessage.primaryBitMap.BitLen(); ind++ {
		if isoMessage.primaryBitMap.GetBit(ind) {
			retVal, err := parseMessage(ind)
			if err == nil {
				isoMessage.fields[ind] = retVal
			} else {
				fmt.Println("Parse error", err.Error())
			}

		}
	}

	for ind := 0; ind < isoMessage.secondaryBitMap.BitLen(); ind++ {
		if isoMessage.secondaryBitMap.GetBit(ind) {
			retVal, err := parseMessage(64 + ind)
			if err == nil {
				isoMessage.fields[64+ind] = retVal
			} else {
				fmt.Println("Parse error", err.Error())
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

	for field := 1; field <= fieldCnt; field++ {
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
