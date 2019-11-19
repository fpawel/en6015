package main

import (
	"fmt"
	"github.com/fpawel/comm/comport"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"os"
	"path/filepath"
	"time"
)

func runMainWindow() {

	settings := walk.NewIniFileSettings("settings.ini")
	defer log.ErrIfFail(settings.Save)

	app := walk.App()
	app.SetOrganizationName("Аналитприбор")
	app.SetProductName("Стенд-6015")
	app.SetSettings(settings)
	log.ErrIfFail(settings.Load)

	var (
		comboBoxTest,
		comboBoxPort,
		comboBoxPortHart *walk.ComboBox
		tableView  *walk.TableView
		mainWindow *walk.MainWindow
		pbStart *walk.PushButton
	)

	taleViewModel := new(taleViewModel)
	taleViewModel.items = []msgLine{}

	err := MainWindow{
		AssignTo:   &mainWindow,
		Title:      "ЭН8800-6602",
		Name:       "MainWindow",
		Font:       Font{PointSize: 12, Family: "Segoe UI"},
		Background: SolidColorBrush{Color: walk.RGB(255, 255, 255)},
		Size:       Size{800, 600},
		Layout:     VBox{},
		Children: []Widget{
			ScrollView{
				VerticalFixed: true,
				Layout:        HBox{},
				Children: []Widget{
					Label{Text: "Исполнение:"},
					ComboBox{
						AssignTo:      &comboBoxTest,
						Model:         works,
						DisplayMember: "Name",
						CurrentIndex:  0,
						MaxSize:Size{280,0},
						MinSize:Size{280,0},
					},
					PushButton{
						AssignTo:&pbStart,
						Text: "Выполнить",
						MaxSize:Size{100,35},
						MinSize:Size{100,35},
						OnClicked: func() {
							taleViewModel.items = []msgLine{}
							taleViewModel.PublishRowsReset()
							workIndex = comboBoxTest.CurrentIndex()
							workName := works[workIndex].Name
							pbStart.SetEnabled(false)
							go func() {
								err := perform()
								if err == nil {
									for _, x := range taleViewModel.items {
										if !x.Ok {
											err = fmt.Errorf(" %s: %s", x.Work, x.Text)
										}
									}
								}
								mainWindow.Synchronize(func() {
									pbStart.SetEnabled(true)
									if err != nil {
										walk.MsgBox(mainWindow, workName, err.Error(), walk.MsgBoxIconError)
									} else {
										walk.MsgBox(mainWindow, workName, "успешно", walk.MsgBoxIconInformation)
									}
								})
							}()
						}},
					Label{Text: "COM порт платы"},
					comboBoxComport(&comboBoxPort, ComportDakKey),
					Label{Text: "COM порт HART модема"},
					comboBoxComport(&comboBoxPortHart, ComportHartKey),
					CheckBox{
						Text: "Пропустить задержку 20 с",
						OnCheckedChanged: func() {
							skipWait20Sec = !skipWait20Sec
						},
					},
				},
			},
			TableView{
				AssignTo:                 &tableView,
				Model:                    taleViewModel,
				Font:                     Font{PointSize: 12, Family: "Segoe UI"},
				NotSortableByHeaderClick: true,
				LastColumnStretched:      true,
				Columns: []TableViewColumn{
					{Title: "Время", Format: "15:04:05", Width: 100},
					{Title: "Проверка", Width: 300},
					{Title: "Результат"},
				},
			},
		},
	}.Create()
	if err != nil {
		panic(err)
	}

	addNewWorkLog = func(work, msg string) {
		taleViewModel.items = append(taleViewModel.items, msgLine{
			Work:      work,
			UpdatedAt: time.Now(),
			Text:      msg,
			Ok:        true,
		})
		tableView.Synchronize(func() {
			taleViewModel.PublishRowsReset()
		})
	}

	printMsg = func(ok bool, msg string) {
		n := len(taleViewModel.items) - 1
		x := &taleViewModel.items[n]
		x.Text = msg
		x.Ok = ok
		x.UpdatedAt = time.Now()
		tableView.Synchronize(func() {
			taleViewModel.PublishRowChanged(n)
		})
		prefix := ""
		if !ok {
			prefix = "ERROR: "
		}
		log.Printf("%s%s\n", prefix, msg)
	}

	mainWindow.Run()
}

type taleViewModel struct {
	walk.TableModelBase
	items []msgLine
}

type msgLine struct {
	UpdatedAt  time.Time
	Work, Text string
	Ok         bool
}

func (m *taleViewModel) RowCount() int {
	return len(m.items)
}

func (m *taleViewModel) Value(row, col int) interface{} {
	x := m.items[row]
	switch col {
	case 0:
		return x.UpdatedAt
	case 1:
		return x.Work
	case 2:
		return x.Text
	default:
		panic("unexpected")
	}
}

func (m *taleViewModel) StyleCell(style *walk.CellStyle) {
	item := m.items[style.Row()]
	switch style.Col() {
	case 0:
		style.TextColor = walk.RGB(0, 120, 0)
	case 1:
		style.TextColor = walk.RGB(0, 0, 128)
	case 2:
		if !item.Ok {
			style.Image = errorPng
			style.TextColor = walk.RGB(255, 0, 0)
		}
	}
}

func newBitmapFromFile(filename string) walk.Image {
	img, err := walk.NewImageFromFile(filepath.Join(filepath.Dir(os.Args[0]), "img", filename))
	if err != nil {
		panic(err)
	}
	return img
}

var (
	errorPng = newBitmapFromFile("error.png")
)

func comboBoxComport(comboBox **walk.ComboBox, key string) ComboBox {
	var comboBox2 *walk.ComboBox
	if comboBox == nil {
		comboBox = &comboBox2
	}

	getComports := func() []string {
		ports, _ := comport.Ports()
		return ports
	}

	comboBoxIndex := func(s string, m []string) int {
		for i, x := range m {
			if s == x {
				return i
			}
		}
		return -1
	}

	comportIndex := func(portName string) int {
		ports, _ := comport.Ports()
		return comboBoxIndex(portName, ports)
	}

	return ComboBox{
		AssignTo:     comboBox,
		Model:        getComports(),
		MaxSize:      Size{100, 0},
		CurrentIndex: comportIndex(iniStr(key)),
		OnMouseDown: func(_, _ int, _ walk.MouseButton) {
			ports := getComports()
			cb := *comboBox
			m := cb.Model().([]string)

			n := cb.CurrentIndex()
			defer func() {
				_ = cb.SetCurrentIndex(n)
			}()
			if len(m) != len(ports) {
				_ = cb.SetModel(ports)
				return
			}
			for i := range m {
				if m[i] != ports[i] {
					_ = cb.SetModel(ports)
					return
				}
			}
		},
		OnCurrentIndexChanged: func() {
			iniPutStr(key, (*comboBox).Text())
		},
	}
}

func iniStr(key string) string {

	s, _ := walk.App().Settings().Get(key)
	return s
}

func iniPutStr(key, value string) {
	if err := walk.App().Settings().Put(key, value); err != nil {
		panic(err)
	}
}
