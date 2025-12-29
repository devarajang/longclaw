package utils

import (
	"encoding/json"
	"math/rand"
	"os"
	"strings"
)

type IsoMsgTemplate struct {
	Message         string `json:"message"`
	OriginalMessage string `jaon:"original_message"`
}

var messageList []IsoMsgTemplate

func LoadTemplates(templatePath string) error {
	messageList = make([]IsoMsgTemplate, 0)

	// Support both file path and directory path for backward compatibility
	filePath := templatePath
	if !strings.HasSuffix(templatePath, ".json") {
		filePath = templatePath + "template_messages.json"
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&messageList)
	if err != nil {
		return err
	}
	return nil
}

func RandomTemplate() IsoMsgTemplate {
	randNum := rand.Intn(len(messageList))
	return messageList[randNum]
}
