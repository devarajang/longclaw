package utils

import (
	"bufio"
	"math/rand"
	"os"
)

var messageList []string

func LoadTemplates(templatePath string) {

	messageList = make([]string, 0)
	file, err := os.Open(templatePath + "template_messages.txt")

	if err != nil {
		panic(err.Error())
	}
	defer file.Close() // Ensure the file is closed
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {

		messageList = append(messageList, scanner.Text())
	}
	//fmt.Println(messageList)
}

func RandomTemplate() string {
	randNum := rand.Intn(len(messageList))
	return messageList[randNum]
}
