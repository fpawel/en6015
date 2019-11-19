package main

import (
	"bytes"
	"context"
	"github.com/fpawel/comm"
	"github.com/fpawel/comm/comport"
	"time"
)

func testHart() error {

	if err := comportHart.SetConfig(comport.Config{
		Name:iniStr(ComportHartKey),
		Baud:        1200,
		ReadTimeout: time.Millisecond,
		Parity:      comport.ParityOdd,
		StopBits:    comport.Stop1,
	}); err != nil {
		return err
	}

	addNewWorkLog("HART: включение", "")

	if err := dakWrite32(0x80, 1000); err != nil {
		printMsg(false, "возможно, HART протокол был включен ранее: " + err.Error())
	} else {
		printMsg(true, "успешно")
	}

	addNewWorkLog("HART: инициализация", "")
	hartID, err := hartInit()
	if err != nil {
		printErr(err)
		return err
	}
	infof("HART: ID=% X", hartID)

	addNewWorkLog("HART: опрос", "")

	for i := 0; i < 3; i++ {
		if b, err := hartReadConcentration(hartID); err != nil {
			errorf(err, "HART: запрос концентрации %d", i+1)
		} else {
			infof("HART: запрос концентрации %d: % X", i+1, b)
			time.Sleep(time.Second)
		}
	}

	addNewWorkLog("HART: отключение", "")
	if err := hartSwitchOff(hartID); err != nil {
		printErr(err)
		return err
	}
	if _,err := read3(1, 0); err != nil {
		return err
	}

	printMsg(true, "успешно")
	return nil

}

func hartInit() ([]byte, error) {
	b, err := hartGetResponse([]byte{
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0x02, 0x00, 0x00, 0x00, 0x02,
	}, func(b []byte) error {
		// 00 01 02 03 04 05 06 07 08 09 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25 26 27 28
		// 06 00 00 18 00 00 FE E2 B4 05 07 01 06 18 00 00 00 01 05 10 00 00 00 60 93 60 93 01 BE
		// 06 00 00 18 00 20 FE E2 B4 05 07 01 06 18 00 00 00 01 05 10 00 00 00 60 93 60 93 01 9E
		// 06 00 00 18 00 00 FE E2 B4 05 07 01 06 18 00 00 00 01 05 10 00 00 00 60 93 60 93 01 BE
		if len(b) != 29 {
			return comm.Err.WithMessagef("ожидалось 29 байт, получено %d: % X", len(b), b)
		}
		if !bytes.Equal(b[:4], []byte{0x06, 0x00, 0x00, 0x18}) {
			return comm.Err.WithMessagef("ожидалось 06 00 00 18, % X", b[:4])
		}
		if b[6] != 0xFE {
			return comm.Err.WithMessage("b[6] == 0xFE")
		}

		if bytes.Equal(b[23:27], []byte{0x60, 0x93, 0x60, 0x93, 0x01}) {
			return comm.Err.WithMessagef("b[29:27] != 60 93 60 93 01, % X", b[23:27])
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return b[15:18], nil
}

func hartGetResponse(req []byte, parse func([]byte) error) ([]byte, error) {
	offset := 0
	response, err := comportHart.NewResponseReader(context.Background(), comm.Config{
		TimeoutEndResponse: 50 * time.Millisecond,
		TimeoutGetResponse: 2 * time.Second,
		MaxAttemptsRead:    5,
	}).GetResponse( req, log, func(request, response []byte) (string, error) {
		var err error
		offset, err = parseHart(response, parse)
		if err != nil {
			return "", err
		}
		return "", nil
	})
	return response[offset:], err
}

func parseHart(response []byte, parse func([]byte) error) (int, error) {

	if len(response) < 5 {
		return 0, comm.Err.WithMessage("длина ответа меньше 5")
	}
	offset := 0
	for i := 2; i < len(response)-1; i++ {
		if response[i] == 0xff && response[i+1] == 0xff && response[i+2] != 0xff {
			offset = i + 2
			break
		}
	}
	if offset == 0 || offset >= len(response) {
		return 0, comm.Err.WithMessage("ответ не соответствует шаблону FF FF XX")
	}
	result := response[offset:]

	if hartCRC(result) != result[len(result)-1] {
		return 0, comm.Err.WithMessage("не совпадает контрольная сумма")
	}
	return offset, parse(result)
}

func hartSwitchOff(hartID []byte) error {
	// 00 01 02 03 04 05 06 07 08 09 10 11 12
	// 82 22 B4 00 00 01 80 04 46 16 00 00 C1
	req := []byte{
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0x82, 0x22, 0xB4,
		hartID[0], hartID[1], hartID[2],
		0x80, 0x04,
		0x46, 0x16, 0x00, 0x00,
		0x00,
	}
	req[5+12] = hartCRC(req[5 : 12+5])

	_, err := hartGetResponse(req, func(b []byte) error {
		// 00 01 02 03 04 05 06 07 08 09 10 11 12 13 14
		// 86 22 B4 00 00 01 80 06 00 00 46 16 00 00 C7
		a := []byte{
			0x86, 0x22, 0xB4, hartID[0], hartID[1], hartID[2], 0x80, 0x06,
		}
		if !bytes.Equal(a, b[:8]) {
			return comm.Err.WithMessagef("ожидалось % X, получено % X", a, b[:8])
		}
		return nil
	})
	return err
}

func hartReadConcentration(id []byte) ([]byte, error) {
	// 00 01 02 03 04 05 06 07 08
	// 82 22 B4 00 00 01 01 00 14
	req := []byte{
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0x82, 0x22, 0xB4,
		id[0], id[1], id[2],
		0x01, 0x00,
		0x82,
	}
	req[8+5] = hartCRC(req[5 : 8+5])

	rpat := []byte{0x86, 0x22, 0xB4, id[0], id[1], id[2], 0x01, 0x07}

	b, err := hartGetResponse(req, func(b []byte) error {
		if len(b) < 16 {
			// нужно сделать паузу, возможно плата тормозит
			//time.Sleep(time.Millisecond * 100)
			return comm.Err.WithMessagef("ожидалось 16 байт, получено % X", b)

		}
		// 00 01 02 03 04 05 06 07 08 09 10 11 12 13 14 15
		// 86 22 B4 00 00 01 01 07 00 00 A1 00 00 00 00 B6
		if !bytes.Equal(rpat, b[:8]) {
			return comm.Err.WithMessagef("ожидалось % X, получено % X", rpat, b)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return b[11:15], nil
}

func hartCRC(b []byte) byte {
	c := b[0]
	for i := 1; i < len(b)-1; i++ {
		c ^= b[i]
	}
	return c
}
