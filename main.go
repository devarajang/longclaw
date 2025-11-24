package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/devarajang/longclaw/database"
	"github.com/devarajang/longclaw/iso"
	network "github.com/devarajang/longclaw/network/server"
	"github.com/devarajang/longclaw/runner"
	"github.com/devarajang/longclaw/utils"
)

func main() {

	basePath := "/Users/deva/workspace/goworkspace/longclaw/"
	dataPath := basePath + "data/"
	certPath := basePath + "certs/server/"

	// Initialize database
	db, err := database.NewStressTestDB(dataPath + "stress_test.db")
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer db.Close()

	utils.LoadTemplates(dataPath)

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
	server, err := network.NewIsoServer(db, certPath)
	if err == nil {
		panic("Unable to create server")
	}
	go server.StartListen()

	str := &runner.StressTestRunner{
		StressChannel: make(chan database.StressTest),
		IsoSpec:       isoSpec,
		Server:        server,
	}

	go str.HandleChannelEvents()

	go func() {
		fmt.Println("Loading test cards")
		utils.LoadCards(dataPath)
	}()

	mux := startHttpHandler(server, db, str)

	http.ListenAndServe(":8080", mux)
	//server.StartStress(5)
}

func startHttpHandler(server *network.IsoServer, db *database.StressTestDB, str *runner.StressTestRunner) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /clients", func(w http.ResponseWriter, r *http.Request) {
		clients := server.GetConnectedClients()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(clients)
	})

	mux.HandleFunc("POST /stress_tests", func(w http.ResponseWriter, r *http.Request) {
		// Create a new stress test (from handler)
		var stressTestReq = database.StressTest{}
		if err := json.NewDecoder(r.Body).Decode(&stressTestReq); err != nil {
			http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
			return
		}
		stressTest, err := db.CreateStressTest(stressTestReq.Name, stressTestReq.TestTimeSecs, stressTestReq.RequestPerSecond)
		if err != nil {
			log.Fatal("Failed to create stress test:", err)
		}
		str.StressChannel <- *stressTest
		log.Printf("Created stress test: %+v\n", stressTest)
		json.NewEncoder(w).Encode(stressTest)
	})
	return mux
}
