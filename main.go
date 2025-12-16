package main

import (
	"fmt"
	"log"

	"github.com/devarajang/longclaw/database"
	"github.com/devarajang/longclaw/handlers"

	"github.com/devarajang/longclaw/iso"
	network "github.com/devarajang/longclaw/network/server"
	"github.com/devarajang/longclaw/runner"
	"github.com/devarajang/longclaw/utils"
)

func main() {

	//	de123 := "030TDAV132218200002140CV0711 322M" // "011TDCV051 613"

	basePath := "/Users/deva/workspace/goworkspace/longclaw/"
	dataPath := basePath + "data/"
	certPath := basePath + "certs/server/"

	/*var App = */

	// Initialize database
	db, err := database.NewStressTestDB(dataPath + "stress_test.db")
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
		return
	}
	defer db.Close()

	err = utils.LoadTemplates(dataPath)

	if err != nil {
		log.Fatal("Failed to initialize message templates:", err)
		return
	}

	isoSpec, err := iso.LoadSpecs(dataPath)
	utils.GlobalIsoSpec = isoSpec

	if err != nil {
		panic(err.Error())
	}

	/*
		isoMessage, err := iso.NewIso8583Message("0420F23E44010EE180720000004001020012164242424242424242000000000000000145031819031648370712030003182602031855410011010000012035077154837077100830063887901000000000638879KWIK PIK MARKET        UKIAH        CAUS015KWIK PIK MARKET840011100000080150170600095482    8400048028022B2IN0120ILKINT120     020048370703181203000000000000000000000000001Z042VD0370000E0810040000000     08210000115077038PR29V0010013025077685145605054230PI0110784483707315790     507715483707 465077685968904      09010005               343", isoSpec)

		if err == nil {
			//fmt.Println(i, isoMessage.FormatPrint())
			fmt.Println(isoMessage.FormatPrint())
			reference := utils.GenerateTimestampID()
			fmt.Println(reference)
			isoMessage.SetField(36, reference)
			fmt.Println(isoMessage.GetField(36))

			msg := isoMessage.FormatIso()
			fmt.Println(msg)
			iso.NewIso8583Message(msg, isoSpec)
			fmt.Println(isoMessage.FormatPrint())
			return
		}
	*/
	isoServer, err := network.NewIsoServer(db, certPath)
	if err == nil {
		panic("Unable to create server")
	}
	go isoServer.StartListen()

	str := &runner.StressTestRunner{
		StressChannel: make(chan database.StressTest),
		IsoSpec:       isoSpec,
		Server:        isoServer,
	}

	go str.HandleChannelEvents()

	go func() {
		fmt.Println("Loading test cards")
		utils.LoadCards(dataPath)
	}()

	var app *handlers.App = &handlers.App{
		Config: &handlers.AppConfig{
			BasePath: basePath,
			DataPath: dataPath,
			CertPath: certPath,
		},
		DB:           db,
		IsoServer:    isoServer,
		StressRunner: str,
	}
	server := handlers.New("1.0", app)
	server.StartServer(":8080")
	//server.StartStress(5)
}
