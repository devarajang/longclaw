package iso

import (
	"encoding/json"
	"errors"
	"os"
)

type FieldDef struct {
	FieldNo          int    `json:"field_no"`
	FieldName        string `json:"field_name"`
	FieldDescription string `json:"field_description"`
	FieldType        string `json:"field_type"`
	MinLength        int    `json:"min_length"`
	MaxLength        int    `json:"max_length"`
}

type IsoSpec struct {
	SpecName         string     `json:"spec_name"`
	FieldDefinitions []FieldDef `json:"field_definitions"`

	FieldDefMap map[int]FieldDef
}

func LoadSpecs(filePath string) (*IsoSpec, error) {
	file, err := os.Open(filePath + "file_spec.json")
	if err != nil {
		return nil, errors.New("Unable to load spec")
	}
	defer file.Close()
	var isoSpec IsoSpec
	isoSpec.FieldDefMap = make(map[int]FieldDef)
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&isoSpec)
	if err != nil {
		panic("Unable to parse json")
	}
	for _, fieldDef := range isoSpec.FieldDefinitions {
		isoSpec.FieldDefMap[fieldDef.FieldNo] = fieldDef
	}
	return &isoSpec, nil

}
