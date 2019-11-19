package main

import (
	"context"
	"fmt"
	"github.com/ansel1/merry"
	"github.com/fpawel/comm"
	"github.com/fpawel/comm/comport"
	"github.com/fpawel/comm/modbus"
	"github.com/powerman/structlog"
	"time"
)

var (
	log = structlog.New()

	addNewWorkLog func(work, msg string)
	printMsg      func(ok bool, msg string)

	skipWait20Sec bool

	comportDak = comport.NewPort(comport.Config{
		Baud:        9600,
		ReadTimeout: time.Millisecond,
	})

	comportHart = new(comport.Port)

	workIndex int

	works = []Work{
		{"687243.620", func() error {
			return testIndicationBoard(76, 74)
		}},
		{"687243.620-01/-02(HART)", func() error {
			if err := testIndicationBoard(76, 74); err != nil {
				return err
			}
			return testHart()
		}},
		{"687243.620-03", func() error {
			return testIndicationBoard(94, 92)
		}},
		{"687243.620-04/-05(HART)", func() error {
			if err := testIndicationBoard(94, 92); err != nil {
				return err
			}
			return testHart()
		}},
		{"HART протокол", testHart},
	}
)



func readerDak() modbus.ResponseReader {
	return comportDak.NewResponseReader(context.Background(), comm.Config{
		TimeoutEndResponse: 30 * time.Millisecond,
		TimeoutGetResponse: time.Second,
		MaxAttemptsRead:    2,
	})
}

type Work struct {
	Name string
	Func func() error
}

func perform() error {
	if err := comportDak.SetConfig(comport.Config{
		Baud:        9600,
		ReadTimeout: time.Millisecond,
		Name:iniStr(ComportDakKey),
	}); err != nil {
		return err
	}
	err := works[workIndex].Func()
	log.ErrIfFail(comportDak.Close)
	log.ErrIfFail(comportHart.Close)
	return err
}

func testIndicationBoard(var3V, var5V modbus.Var) error {

	addNewWorkLog("связь", "")

	if err := dakWrite32(80, 0); err != nil {
		return err
	}
	if err := setupPin(false); err != nil {
		return err
	}

	if !skipWait20Sec {
		printMsg(true, "установка режимов работы, 20 c")
		time.Sleep(20 * time.Second)
	}

	addNewWorkLog("напряжение питания 3В", "")
	if err := checkValue3(1, var3V, 3.2, 3.4); err != nil {
		return err
	}

	addNewWorkLog("напряжение питания 5В", "")
	if err := checkValue3(1, var5V, 4.9, 5.1); err != nil {
		return err
	}

	addNewWorkLog("нагрев", "")
	if err := setupPin(true); err != nil {
		return err
	}
	defer func() {
		_ = doSetupPin(false)
	}()

	printMsg(true, "выдержка нагревателя, 5 c")
	time.Sleep(5 * time.Second)

	if err := checkValue3(105, 14, 900, 1300); err != nil {
		return err
	}

	addNewWorkLog("отключить нагрев", "")
	if err := setupPin(false); err != nil {
		return err
	}

	for _, currOut := range []float64{4, 12, 22} {

		addNewWorkLog(fmt.Sprintf("токовый выход %v мА", currOut), "")
		if err := dakWrite32(83, currOut); err != nil {
			return err
		}

		infof("установка токового выхода %v мА, 5 c", currOut)
		time.Sleep(5 * time.Second)

		u := currOut * 100

		if err := checkValue3(105, 12, u*0.98, u*1.02); err != nil {
			return err
		}

		addNewWorkLog(fmt.Sprintf("токовый выход %v мА: пульсации", currOut), "")

		if err := checkValue3(105, 16, 0, 125); err != nil {
			return err
		}
	}
	return nil
}

func checkValue3(addr modbus.Addr, devVar modbus.Var, min, max float64) error {
	value, err := read3(addr, devVar)
	if err != nil {
		return err
	}
	what := "ДАК"
	if addr == 105 {
		what = "стенд"
	}
	printf(value >= min && value <= max,
		"%s%d: контроль %v...%v : %v", what, devVar, min, max, value)
	return err
}

func read3(addr modbus.Addr, reg modbus.Var) (float64, error) {
	what := "стенд"
	if addr == 1 {
		what = "ДАК"
	}
	value, err := modbus.Read3BCD(log, readerDak(), addr, reg)
	if err != nil {
		errorf(err, "%s%d", what, reg)
		return 0, merry.Appendf(err, "%s%d", what, reg)
	}
	infof("%s%d=%v", what, reg, value)
	return value, nil
}

func dakWrite32(devCmd modbus.DevCmd, value float64) error {
	infof("ДАК: запись %d, %v", devCmd, value)
	err := modbus.Write32(log, readerDak(), 1, 0x16, devCmd, value)
	if err != nil {
		errorf(err, "ДАК: запись %d, %v", devCmd, value)
		return merry.Appendf(err, "ДАК: запись %d, %v", devCmd, value)
	}
	return nil
}

func setupPin(on bool) error {
	s := "включить"
	if !on {
		s = "выключить"
	}
	s += " нагрев платы"
	printInfo(s)
	err := doSetupPin(on)
	if err != nil {
		printErr(err)
		return merry.Append(err, s)
	}
	return nil
}

func doSetupPin(on bool) error {
	req := modbus.Request{
		Addr:     105,
		ProtoCmd: 4,
		Data:     []byte{0},
	}
	if on {
		req.Data[0] = 1
	}
	_, err := req.GetResponse(log, readerDak(), nil)
	return err
}

func infof(format string, args ...interface{}) {
	printf(true, format, args...)
}

func printInfo(msg string) {
	printMsg(true, msg)
}

func printf(ok bool, format string, args ...interface{}) {
	printMsg(ok, fmt.Sprintf(format, args...))
}

func printErr(err error) {
	printMsg(false, err.Error())
}

func errorf(err error, format string, args ...interface{}) {
	printMsg(false, fmt.Sprintf(format, args...)+": "+err.Error())
}
