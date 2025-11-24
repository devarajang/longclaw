package runner

import (
	"fmt"

	"github.com/devarajang/longclaw/database"
	"github.com/devarajang/longclaw/iso"
	network "github.com/devarajang/longclaw/network/server"
)

type StressTestRunner struct {
	StressChannel chan database.StressTest
	IsoSpec       *iso.IsoSpec
	Server        *network.IsoServer
}

func (str *StressTestRunner) HandleChannelEvents() {

	/*for i := range 1 {

	isoMessage, err := iso.NewIso8583Message(utils.RandomTemplate(), str.IsoSpec)

	if err == nil {
		//fmt.Println(i, isoMessage.FormatPrint())
		fmt.Println(i, isoMessage.FormatIso())
	} else {
		fmt.Println(err.Error())
	}
	}*/

	for {
		stressTest := <-str.StressChannel
		fmt.Println("Received on channel", stressTest)
		str.Server.RunStress(stressTest, str.IsoSpec)
	}

}
