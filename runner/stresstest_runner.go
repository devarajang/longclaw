package runner

import (
	"fmt"
	"sync/atomic"

	"github.com/devarajang/longclaw/internal/domain"
	"github.com/devarajang/longclaw/iso"
	network "github.com/devarajang/longclaw/network/server"
)

type StressTestRunner struct {
	StressChannel chan domain.StressTest
	IsoSpec       *iso.IsoSpec
	Server        *network.IsoServer
	busy          atomic.Bool // held from Reserve until the reserved test finishes
}

// Reserve claims the runner for one test and returns false if another test holds it.
// After a successful Reserve the caller must either send the test on StressChannel or call Release.
func (str *StressTestRunner) Reserve() bool {
	return str.busy.CompareAndSwap(false, true)
}

// Release frees the runner without running a test
func (str *StressTestRunner) Release() {
	str.busy.Store(false)
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
		str.Release()
	}

}
