package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/devarajang/longclaw/database"
	"github.com/devarajang/longclaw/handlers"
	"github.com/devarajang/longclaw/internal/config"
	"github.com/devarajang/longclaw/internal/domain"

	"github.com/devarajang/longclaw/iso"
	network "github.com/devarajang/longclaw/network/server"
	"github.com/devarajang/longclaw/runner"
	"github.com/devarajang/longclaw/utils"
)

func main() {
	basePath, err := resolveBasePath()
	if err != nil {
		log.Fatal("Failed to resolve base path:", err)
	}
	log.Println("Using base path", basePath)

	cfg, err := config.Load(basePath)
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// Initialize database
	db, err := database.NewStressTestDB(cfg.Database.Path)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
		return
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("Failed to close database: %v", closeErr)
		}
	}()

	err = utils.LoadTemplates(cfg.Data.TemplatesPath)

	if err != nil {
		log.Fatal("Failed to initialize message templates:", err)
		return
	}

	isoSpec, err := iso.LoadSpecsFromFile(cfg.Data.SpecPath)
	if err != nil {
		log.Fatal("Failed to load ISO spec:", err)
	}
	utils.GlobalIsoSpec = isoSpec

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
	log.Println("Starting New ISO Server")
	isoServer, err := network.NewIsoServer(db, cfg)
	if err != nil {
		log.Fatal("Unable to create server:", err)
	}
	go func() {
		if err := isoServer.StartListen(); err != nil {
			log.Fatal("ISO server failed to start:", err)
		}
	}()

	str := &runner.StressTestRunner{
		StressChannel: make(chan domain.StressTest),
		IsoSpec:       isoSpec,
		Server:        isoServer,
	}

	go str.HandleChannelEvents()

	go func() {
		log.Println("Loading test cards")
		if err := utils.LoadCards(cfg.Data.CardsPath); err != nil {
			log.Fatal("Failed to load test cards:", err)
		}
	}()

	var app *handlers.App = &handlers.App{
		Config: &handlers.AppConfig{
			BasePath: basePath,
			DataPath: cfg.Data.Path,
			CertPath: cfg.TLS.ServerCertPath,
		},
		DB:           db,
		IsoServer:    isoServer,
		StressRunner: str,
	}
	server := handlers.New("1.0", app)
	if err := server.StartServer(cfg.Server.HTTPPort); err != nil {
		log.Fatal("HTTP server failed to start:", err)
	}
	//server.StartStress(5)
}

func resolveBasePath() (string, error) {
	if basePath := os.Getenv("LONGCLAW_BASE_PATH"); basePath != "" {
		return filepath.Abs(basePath)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	return filepath.Abs(workingDir)
}
