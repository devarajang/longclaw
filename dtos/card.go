package dtos

type Card struct {
	CardNumber   string  `json:"card_number"`
	CVV2         string  `json:"cvv2"`
	Track2Data   string  `json:"track2_data"`
	PIN          string  `json:"pin"`
	ExpiryDate   string  `json:"expiry_date"`
	AddressLine1 *string `json:"address_line1"`
	ZipCode      *string `json:"zip_code"`
	TestDataName string  `json:"test_data_name"`
}
