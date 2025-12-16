package utils

import (
	"encoding/json"
	"math/rand"
	"os"
)

type IsoMsgTemplate struct {
	Message         string `json:"message"`
	OriginalMessage string `jaon:"original_message"`
}

var messageList []IsoMsgTemplate

func LoadTemplates(templatePath string) error {

	messageList = make([]IsoMsgTemplate, 0)
	file, err := os.Open(templatePath + "template_messages.json")

	if err != nil {
		panic(err.Error())
	}
	defer file.Close() // Ensure the file is closed
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&messageList)

	if err != nil {
		return err
	}
	return nil
	/*scanner := bufio.NewScanner(file)

	for scanner.Scan() {

		messageList = append(messageList, scanner.Text())
		}*/
	//fmt.Println(messageList)
}

func RandomTemplate() IsoMsgTemplate {
	randNum := rand.Intn(len(messageList))
	return messageList[randNum]
}
