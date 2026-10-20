package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	seriallib "go.bug.st/serial"

	"github.com/jovil96/balanza-agent/internal/config"
	"github.com/jovil96/balanza-agent/internal/domain"
	"github.com/jovil96/balanza-agent/internal/printer"
	"github.com/jovil96/balanza-agent/internal/reader"
	"github.com/jovil96/balanza-agent/internal/scale"
	serialreader "github.com/jovil96/balanza-agent/internal/serial"
)

// ScaleReader is the subset of reader capabilities used by the desktop UI.
type ScaleReader interface {
	Status() <-chan reader.Status
	Reconfigure(ctx context.Context, cfg *config.Config) error
}

type fields struct {
	scalePort, scaleBaud, scaleData, scaleStop, timeout, pattern, unit *walk.LineEdit
	usbPort, usbBaud, usbData, usbStop, usbTimeout                     *walk.LineEdit
	tcpHost, tcpPort, tcpTimeout                                       *walk.LineEdit
	printerPort, printerBaud, printerData, printerStop                 *walk.LineEdit
	company, ruc, activity, operator, title, footer                    *walk.LineEdit
	apiAddress, apiOrigins, apiKey                                     *walk.LineEdit
	connType, scaleModel, scaleParity, usbParity, printerParity        *walk.ComboBox
	apiEnabled                                                         *walk.CheckBox
}

type ticketCounter interface {
	PeekTicketNumber() (int64, error)
	NextTicketNumber() (int64, error)
	ListProducts() ([]string, error)
	AddProduct(string) error
	DeleteProduct(string) error
}

func Run(ctx context.Context, cfgPath string, cfg *config.Config, reader ScaleReader, service *scale.Service, pos *printer.Printer, counter ticketCounter) error {
	serialCtx := ctx
	var appendLog func(string)
	var mw *walk.MainWindow
	var weight, unit, status, connectionState, lastReading, clock *walk.Label
	var raw, ticketNumber, operator, activity, newProduct, note *walk.LineEdit
	var logBox *walk.TextEdit
	var product *walk.ComboBox
	var printButton *walk.PushButton
	var f fields
	var latest *domain.Reading
	parities := []string{"N", "E", "O"}
	apiURL := "http://" + cfg.API.Address + "/api/v1/reading"
	nextNumber, err := counter.PeekTicketNumber()
	if err != nil {
		return err
	}
	products, err := counter.ListProducts()
	if err != nil {
		return err
	}
	saveSettings := func() {
		if err := saveForm(cfgPath, cfg, f); err != nil {
			walk.MsgBox(mw, "Configuración inválida", err.Error(), walk.MsgBoxIconError)
			return
		}
		pos.UpdateConfig(cfg.Printer)
		operator.SetText(cfg.Printer.Operator)
		activity.SetText(cfg.Printer.Activity)
		if err := reader.Reconfigure(serialCtx, cfg); err != nil {
			walk.MsgBox(mw, "Error al reconectar", err.Error(), walk.MsgBoxIconError)
			return
		}
		appendLog("↺ Configuración guardada — reconectando al puerto " + cfg.Serial.Port + "...")
		walk.MsgBox(mw, "Configuración guardada", "Reconectando a la balanza con los nuevos parámetros...", walk.MsgBoxIconInformation)
	}
	// minimizeToTray tracks whether the window close should hide instead of exit.
	minimizeToTray := true

	err = (MainWindow{
		AssignTo: &mw, Title: "Balanza POS", MinSize: Size{Width: 880, Height: 650}, Size: Size{Width: 1020, Height: 680},
		Font: Font{Family: "Segoe UI", PointSize: 10}, Background: SolidColorBrush{Color: walk.RGB(239, 245, 249)},
		Layout: VBox{MarginsZero: true, SpacingZero: true}, Children: []Widget{
			Composite{Background: SolidColorBrush{Color: walk.RGB(16, 73, 113)}, MinSize: Size{Height: 72}, MaxSize: Size{Height: 72}, Layout: HBox{Margins: Margins{Left: 24, Top: 10, Right: 24, Bottom: 10}, Spacing: 14}, Children: []Widget{
				Label{Text: "BALANZA", TextColor: walk.RGB(186, 230, 253), Font: Font{Family: "Segoe UI Semibold", PointSize: 14}, MinSize: Size{Width: 90}},
				Composite{Layout: VBox{MarginsZero: true, Spacing: 1}, Children: []Widget{
					Label{Text: "Balanza POS", TextColor: walk.RGB(255, 255, 255), Font: Font{Family: "Segoe UI Semibold", PointSize: 18}},
					Label{Text: "Conexión directa con balanza", TextColor: walk.RGB(203, 225, 238)},
				}}, HSpacer{},
				Label{AssignTo: &clock, Text: time.Now().Format("02 Jan 2006  |  15:04"), TextColor: walk.RGB(226, 238, 246)},
			}},
			TabWidget{Pages: []TabPage{
				{Title: "Pesaje", Background: SolidColorBrush{Color: walk.RGB(239, 245, 249)}, Layout: VBox{Margins: Margins{Left: 18, Top: 12, Right: 18, Bottom: 12}, Spacing: 8}, Children: []Widget{
					Composite{MinSize: Size{Height: 190}, MaxSize: Size{Height: 190}, Layout: HBox{MarginsZero: true, Spacing: 10}, Children: []Widget{
						Composite{Background: SolidColorBrush{Color: walk.RGB(12, 82, 128)}, MinSize: Size{Width: 540, Height: 190}, MaxSize: Size{Height: 190}, Layout: VBox{Margins: Margins{Left: 24, Top: 16, Right: 24, Bottom: 16}, Spacing: 6}, Children: []Widget{
							Label{Text: "PESO ACTUAL", TextColor: walk.RGB(191, 225, 244), Font: Font{Family: "Segoe UI Semibold", PointSize: 12}},
							Composite{Layout: HBox{MarginsZero: true, Spacing: 12}, Children: []Widget{
								Label{AssignTo: &weight, Text: "0.000", TextAlignment: AlignFar, TextColor: walk.RGB(255, 255, 255), Font: Font{Family: "Segoe UI Semibold", PointSize: 42}},
								Label{AssignTo: &unit, Text: cfg.Parser.DefaultUnit, TextAlignment: AlignNear, TextColor: walk.RGB(216, 237, 249), MinSize: Size{Width: 74}, Font: Font{Family: "Segoe UI Semibold", PointSize: 21}},
							}},
							Label{AssignTo: &status, Text: "Esperando lectura de la balanza...", TextAlignment: AlignCenter, TextColor: walk.RGB(255, 224, 151), Font: Font{Family: "Segoe UI Semibold", PointSize: 10}},
						}},
						GroupBox{Title: "Estado de conexión", MinSize: Size{Width: 330, Height: 190}, MaxSize: Size{Height: 190}, Layout: Grid{Columns: 2, Margins: Margins{Left: 14, Top: 10, Right: 14, Bottom: 10}, Spacing: 7}, Children: []Widget{
							Label{Text: "Estado"}, Label{AssignTo: &connectionState, Text: "Conectando...", TextColor: walk.RGB(180, 83, 9)},
							Label{Text: "Puerto"}, Label{Text: cfg.Serial.Port, TextAlignment: AlignFar},
							Label{Text: "API local"}, Label{Text: apiURL, TextColor: walk.RGB(37, 99, 235), ToolTipText: apiURL},
							Label{Text: "Última lectura"}, Label{AssignTo: &lastReading, Text: "--", TextAlignment: AlignFar},
						}},
					}},
					GroupBox{Title: "Diagnóstico", MinSize: Size{Height: 130}, MaxSize: Size{Height: 130}, Layout: VBox{Margins: Margins{Left: 12, Top: 8, Right: 12, Bottom: 10}, Spacing: 5}, Children: []Widget{
						Composite{Layout: HBox{MarginsZero: true, Spacing: 8}, Children: []Widget{
							Label{Text: "Última trama:", Font: Font{Family: "Segoe UI Semibold", PointSize: 10}},
							LineEdit{AssignTo: &raw, ReadOnly: true},
							PushButton{Text: "Limpiar log", MinSize: Size{Width: 110, Height: 28}, OnClicked: func() {
								raw.SetText("")
								logBox.SetText("")
							}},
						}},
						TextEdit{AssignTo: &logBox, ReadOnly: true, VScroll: true,
							Font:       Font{Family: "Consolas", PointSize: 8},
							Background: SolidColorBrush{Color: walk.RGB(15, 23, 42)},
							TextColor:  walk.RGB(148, 213, 252),
						},
					}},
					GroupBox{Title: "Datos del ticket", MinSize: Size{Height: 190}, MaxSize: Size{Height: 190}, Layout: Grid{Columns: 4, Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 12}, Spacing: 8}, Children: []Widget{
						Label{Text: "Ticket N°"}, LineEdit{AssignTo: &ticketNumber, Text: formatTicketNumber(nextNumber), ReadOnly: true}, Label{Text: "Operador"}, LineEdit{AssignTo: &operator, Text: cfg.Printer.Operator, ReadOnly: true},
						Label{Text: "Actividad"}, LineEdit{AssignTo: &activity, Text: cfg.Printer.Activity, ReadOnly: true}, Label{Text: "Producto"}, ComboBox{AssignTo: &product, Model: products, CurrentIndex: 0},
						Label{Text: "Nuevo producto"}, LineEdit{AssignTo: &newProduct}, Label{Text: "Catálogo"}, Composite{Layout: HBox{MarginsZero: true, Spacing: 8}, Children: []Widget{
							PushButton{Text: "Agregar", MinSize: Size{Width: 90, Height: 34}, OnClicked: func() {
								name := strings.TrimSpace(newProduct.Text())
								if err := counter.AddProduct(name); err != nil {
									walk.MsgBox(mw, "Producto inválido", err.Error(), walk.MsgBoxIconWarning)
									return
								}
								products, err = counter.ListProducts()
								if err != nil {
									walk.MsgBox(mw, "Productos", err.Error(), walk.MsgBoxIconError)
									return
								}
								_ = product.SetModel(products)
								_ = product.SetCurrentIndex(productIndex(products, name))
								newProduct.SetText("")
							}},
							PushButton{Text: "Quitar", MinSize: Size{Width: 90, Height: 34}, OnClicked: func() {
								if len(products) <= 1 {
									walk.MsgBox(mw, "Productos", "Debe conservar al menos un producto.", walk.MsgBoxIconWarning)
									return
								}
								name := strings.TrimSpace(product.Text())
								if name == "" {
									return
								}
								if walk.MsgBox(mw, "Quitar producto", "¿Quitar "+name+" del catálogo?", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
									return
								}
								if err := counter.DeleteProduct(name); err != nil {
									walk.MsgBox(mw, "Productos", err.Error(), walk.MsgBoxIconError)
									return
								}
								products, err = counter.ListProducts()
								if err != nil {
									walk.MsgBox(mw, "Productos", err.Error(), walk.MsgBoxIconError)
									return
								}
								_ = product.SetModel(products)
								_ = product.SetCurrentIndex(0)
							}},
						}},
						Label{Text: "Nota / referencia"}, LineEdit{AssignTo: &note},
					}},
					Composite{MinSize: Size{Height: 50}, MaxSize: Size{Height: 50}, Layout: HBox{MarginsZero: true, Spacing: 10}, Children: []Widget{
						PushButton{Text: "Nuevo", MinSize: Size{Width: 140, Height: 50}, OnClicked: func() {
							for _, field := range []*walk.LineEdit{note, raw} {
								field.SetText("")
							}
							note.SetFocus()
						}}, HSpacer{},
						PushButton{AssignTo: &printButton, Text: "Vista previa e imprimir", Enabled: false, MinSize: Size{Width: 360, Height: 46}, Font: Font{Family: "Segoe UI Semibold", PointSize: 11}, OnClicked: func() {
							if latest == nil {
								return
							}
							data, err := ticketData(latest, ticketNumber, operator, product.Text(), note)
							if err != nil {
								walk.MsgBox(mw, "Datos incompletos", err.Error(), walk.MsgBoxIconWarning)
								return
							}
							if walk.MsgBox(mw, "Vista previa del ticket", pos.Preview(data)+"\n\n¿Confirmar impresión?", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
								return
							}
							reserved, err := counter.NextTicketNumber()
							if err != nil {
								walk.MsgBox(mw, "No se pudo numerar el ticket", err.Error(), walk.MsgBoxIconError)
								return
							}
							data.Number = formatTicketNumber(reserved)
							if err := pos.PrintTicket(data); err != nil {
								walk.MsgBox(mw, "No se pudo imprimir", err.Error(), walk.MsgBoxIconError)
								return
							}
							walk.MsgBox(mw, "Impresión", "Ticket enviado correctamente.", walk.MsgBoxIconInformation)
							ticketNumber.SetText(formatTicketNumber(reserved + 1))
						}},
					}},
					VSpacer{},
				}},
				{Title: "Configuración", Background: SolidColorBrush{Color: walk.RGB(239, 245, 249)}, Layout: VBox{Margins: Margins{Left: 24, Top: 20, Right: 24, Bottom: 20}, Spacing: 9}, Children: configWidgets(cfg, &f, parities, func() { showPorts(mw) }, saveSettings)},
				{Title: "Integración web", Background: SolidColorBrush{Color: walk.RGB(239, 245, 249)}, Layout: VBox{Margins: Margins{Left: 24, Top: 20, Right: 24, Bottom: 20}, Spacing: 10}, Children: integrationWidgets(cfg, &f, saveSettings)},
			}},
		},
	}).Create()
	if err != nil {
		return err
	}

	// --- System tray setup ---
	ni, err := setupTray(mw, &minimizeToTray)
	if err != nil {
		return err
	}
	defer ni.Dispose()

	appendLog = func(line string) {
		ts := time.Now().Format("15:04:05")
		entry := "[" + ts + "] " + line + "\r\n"
		current := logBox.Text()
		// mantener máximo 200 líneas para no acumular sin límite
		lines := strings.Split(current, "\n")
		if len(lines) > 200 {
			current = strings.Join(lines[len(lines)-200:], "\n")
		}
		logBox.SetText(current + entry)
		// scroll al final
		logBox.SendMessage(0x00B1, 0x7FFFFFFF, 0) // EM_SETSEL al final
		logBox.SendMessage(0x00B7, 0, 0)           // EM_SCROLLCARET
	}

	readings, unsubscribe := service.Subscribe()
	defer unsubscribe()
	clockTicker := time.NewTicker(time.Second)
	defer clockTicker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-clockTicker.C:
				mw.Synchronize(func() { clock.SetText(now.Format("02 Jan 2006  |  15:04")) })
			case connection := <-reader.Status():
				mw.Synchronize(func() {
					status.SetText(connection.Message)
					connectionState.SetText(connection.Message)
					if connection.Connected {
						status.SetTextColor(walk.RGB(186, 230, 253))
						connectionState.SetTextColor(walk.RGB(21, 128, 61))
						appendLog("✔ " + connection.Message)
					} else {
						status.SetTextColor(walk.RGB(254, 202, 202))
						connectionState.SetTextColor(walk.RGB(185, 28, 28))
						printButton.SetEnabled(false)
						appendLog("✖ " + connection.Message)
					}
				})
			case reading := <-readings:
				r := reading
				mw.Synchronize(func() {
					latest = &r
					weight.SetText(fmt.Sprintf("%.3f", r.Weight))
					unit.SetText(r.Unit)
					raw.SetText(r.Raw)
					lastReading.SetText(r.Timestamp.Format("15:04:05"))
					connectionState.SetText("Balanza conectada")
					connectionState.SetTextColor(walk.RGB(21, 128, 61))
					if r.Stable {
						status.SetText("Lectura estable")
						status.SetTextColor(walk.RGB(187, 247, 208))
						appendLog(fmt.Sprintf("● %.3f %s  [estable]  raw: %s", r.Weight, r.Unit, r.Raw))
					} else {
						status.SetText("Lectura inestable")
						status.SetTextColor(walk.RGB(254, 224, 151))
						appendLog(fmt.Sprintf("○ %.3f %s  [inestable]  raw: %s", r.Weight, r.Unit, r.Raw))
					}
					printButton.SetEnabled(true)
				})
			}
		}
	}()
	mw.Run()
	return nil
}

func formatTicketNumber(number int64) string {
	return fmt.Sprintf("%08d", number)
}

func productIndex(products []string, selected string) int {
	for index, product := range products {
		if strings.EqualFold(product, selected) {
			return index
		}
	}
	if len(products) > 0 {
		return 0
	}
	return -1
}

func ticketData(reading *domain.Reading, number, operator *walk.LineEdit, product string, note *walk.LineEdit) (printer.Ticket, error) {
	if strings.TrimSpace(operator.Text()) == "" {
		return printer.Ticket{}, fmt.Errorf("configure y guarde el operador en la pestaña Configuración")
	}
	if strings.TrimSpace(product) == "" {
		return printer.Ticket{}, fmt.Errorf("seleccione un producto")
	}
	return printer.Ticket{
		Number: strings.TrimSpace(number.Text()), Weight: reading.Weight, Unit: reading.Unit,
		Operator: strings.TrimSpace(operator.Text()), Product: strings.TrimSpace(product),
		Note: strings.TrimSpace(note.Text()), Created: time.Now(),
	}, nil
}

func runLegacy(ctx context.Context, cfgPath string, cfg *config.Config, reader *serialreader.Reader, service *scale.Service, pos *printer.Printer) error {
	var mw *walk.MainWindow
	var weight, unit, status *walk.Label
	var raw, operator, note *walk.LineEdit
	var printButton *walk.PushButton
	var f fields
	var latest *domain.Reading
	parities := []string{"N", "E", "O"}

	err := (MainWindow{
		AssignTo: &mw, Title: "Balanza POS", MinSize: Size{Width: 760, Height: 600}, Size: Size{Width: 860, Height: 680},
		Font:       Font{Family: "Segoe UI", PointSize: 10},
		Background: SolidColorBrush{Color: walk.RGB(244, 247, 250)},
		Layout:     VBox{Margins: Margins{Left: 20, Top: 20, Right: 20, Bottom: 20}},
		Children: []Widget{TabWidget{Pages: []TabPage{
			{Title: "Pesaje", Background: SolidColorBrush{Color: walk.RGB(244, 247, 250)}, Layout: VBox{Margins: Margins{Left: 24, Top: 24, Right: 24, Bottom: 24}, Spacing: 16}, Children: []Widget{
				Label{Text: "MONITOR DE PESAJE", TextColor: walk.RGB(30, 41, 59), Font: Font{Family: "Segoe UI Semibold", PointSize: 16}},
				Label{Text: "Lectura recibida directamente desde la balanza", TextColor: walk.RGB(71, 85, 105)},
				Label{Text: "API local: http://" + cfg.API.Address + "/api/v1/reading", TextColor: walk.RGB(37, 99, 235)},
				Composite{Background: SolidColorBrush{Color: walk.RGB(15, 74, 110)}, MinSize: Size{Height: 150}, Layout: HBox{Margins: Margins{Left: 24, Top: 28, Right: 24, Bottom: 28}}, Children: []Widget{
					Label{AssignTo: &weight, Text: "0.000", TextAlignment: AlignFar, TextColor: walk.RGB(255, 255, 255), Font: Font{Family: "Segoe UI Semibold", PointSize: 38}},
					Label{AssignTo: &unit, Text: cfg.Parser.DefaultUnit, TextAlignment: AlignNear, TextColor: walk.RGB(186, 230, 253), MinSize: Size{Width: 90}, Font: Font{Family: "Segoe UI Semibold", PointSize: 20}},
				}},
				Label{AssignTo: &status, Text: "● Esperando datos de la balanza...", TextColor: walk.RGB(180, 83, 9), Font: Font{Family: "Segoe UI Semibold", PointSize: 10}},
				GroupBox{Title: "Diagnóstico", Layout: VBox{Margins: Margins{Left: 16, Top: 12, Right: 16, Bottom: 14}}, Children: []Widget{
					Label{Text: "Trama completa recibida"}, LineEdit{AssignTo: &raw, ReadOnly: true},
				}},
				GroupBox{Title: "Datos del ticket", Layout: Grid{Columns: 2, Margins: Margins{Left: 16, Top: 12, Right: 16, Bottom: 14}, Spacing: 10}, Children: []Widget{
					Label{Text: "Operador"}, LineEdit{AssignTo: &operator},
					Label{Text: "Nota / referencia"}, LineEdit{AssignTo: &note},
				}},
				PushButton{AssignTo: &printButton, Text: "Imprimir ticket", Enabled: false, MinSize: Size{Height: 46}, Font: Font{Family: "Segoe UI Semibold", PointSize: 11}, OnClicked: func() {
					if latest == nil {
						return
					}
					if err := pos.PrintTicket(printer.Ticket{Weight: latest.Weight, Unit: latest.Unit, Operator: operator.Text(), Note: note.Text(), Created: time.Now()}); err != nil {
						walk.MsgBox(mw, "No se pudo imprimir", err.Error(), walk.MsgBoxIconError)
						return
					}
					walk.MsgBox(mw, "Impresión", "Ticket enviado correctamente.", walk.MsgBoxIconInformation)
				}},
			}},
			{Title: "Configuración", Layout: VBox{Margins: Margins{Left: 24, Top: 20, Right: 24, Bottom: 20}, Spacing: 9}, Children: configWidgets(cfg, &f, parities, func() { showPorts(mw) }, func() {
				if err := saveForm(cfgPath, cfg, f); err != nil {
					walk.MsgBox(mw, "Configuración inválida", err.Error(), walk.MsgBoxIconError)
					return
				}
				walk.MsgBox(mw, "Configuración guardada", "Los cambios se aplicarán al reiniciar la aplicación.", walk.MsgBoxIconInformation)
			})},
		}}},
	}).Create()
	if err != nil {
		return err
	}
	readings, unsubscribe := service.Subscribe()
	defer unsubscribe()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case connection := <-reader.Status():
				mw.Synchronize(func() {
					status.SetText("● " + connection.Message)
					if connection.Connected {
						status.SetTextColor(walk.RGB(37, 99, 235))
					} else {
						status.SetTextColor(walk.RGB(185, 28, 28))
						printButton.SetEnabled(false)
					}
				})
			case reading := <-readings:
				r := reading
				mw.Synchronize(func() {
					latest = &r
					weight.SetText(fmt.Sprintf("%.3f", r.Weight))
					unit.SetText(r.Unit)
					raw.SetText(r.Raw)
					if r.Stable {
						status.SetText("● Conectada · lectura estable")
						status.SetTextColor(walk.RGB(21, 128, 61))
					} else {
						status.SetText("● Conectada · lectura inestable")
						status.SetTextColor(walk.RGB(180, 83, 9))
					}
					printButton.SetEnabled(true)
				})
			}
		}
	}()
	mw.Run()
	return nil
}

func configWidgets(cfg *config.Config, f *fields, parities []string, detect, save func()) []Widget {
	models := []string{"Genérica / CAS-Digi", "Lexus Matrix Zero", "CAS CL5200N"}
	connTypes := []string{"RS-232 / Serial (COM)", "USB-CDC (COM virtual)", "Ethernet / TCP"}
	return []Widget{
		Label{Text: "CONFIGURACIÓN DE DISPOSITIVOS", TextColor: walk.RGB(30, 41, 59), Font: Font{Family: "Segoe UI Semibold", PointSize: 16}},
		Label{Text: "Selecciona el tipo de conexión con la balanza y configura los parámetros.", TextColor: walk.RGB(71, 85, 105)},

		GroupBox{Title: "Tipo de conexión", Layout: Grid{Columns: 2, Margins: Margins{Left: 16, Top: 14, Right: 16, Bottom: 16}, Spacing: 10}, Children: []Widget{
			Label{Text: "Interfaz"},
			ComboBox{AssignTo: &f.connType, Model: connTypes, CurrentIndex: connTypeIndex(cfg.Connection.Type)},
		}},

		GroupBox{Title: "RS-232 / Serial", Layout: Grid{Columns: 4, Margins: Margins{Left: 16, Top: 14, Right: 16, Bottom: 16}, Spacing: 10}, Children: []Widget{
			Label{Text: "Puerto COM"}, LineEdit{AssignTo: &f.scalePort, Text: cfg.Serial.Port},
			Label{Text: "Velocidad"}, LineEdit{AssignTo: &f.scaleBaud, Text: strconv.Itoa(cfg.Serial.BaudRate)},
			Label{Text: "Bits de datos"}, LineEdit{AssignTo: &f.scaleData, Text: strconv.Itoa(cfg.Serial.DataBits)},
			Label{Text: "Paridad"}, ComboBox{AssignTo: &f.scaleParity, Model: parities, CurrentIndex: parityIndex(cfg.Serial.Parity)},
			Label{Text: "Bits de parada"}, LineEdit{AssignTo: &f.scaleStop, Text: strconv.Itoa(cfg.Serial.StopBits)},
			Label{Text: "Timeout (ms)"}, LineEdit{AssignTo: &f.timeout, Text: strconv.Itoa(cfg.Serial.ReadTimeoutMs)},
		}},

		GroupBox{Title: "USB-CDC (puerto COM virtual)", Layout: Grid{Columns: 4, Margins: Margins{Left: 16, Top: 14, Right: 16, Bottom: 16}, Spacing: 10}, Children: []Widget{
			Label{Text: "Puerto COM"}, LineEdit{AssignTo: &f.usbPort, Text: cfg.USB.Port},
			Label{Text: "Velocidad"}, LineEdit{AssignTo: &f.usbBaud, Text: strconv.Itoa(cfg.USB.BaudRate)},
			Label{Text: "Bits de datos"}, LineEdit{AssignTo: &f.usbData, Text: strconv.Itoa(cfg.USB.DataBits)},
			Label{Text: "Paridad"}, ComboBox{AssignTo: &f.usbParity, Model: parities, CurrentIndex: parityIndex(cfg.USB.Parity)},
			Label{Text: "Bits de parada"}, LineEdit{AssignTo: &f.usbStop, Text: strconv.Itoa(cfg.USB.StopBits)},
			Label{Text: "Timeout (ms)"}, LineEdit{AssignTo: &f.usbTimeout, Text: strconv.Itoa(cfg.USB.ReadTimeoutMs)},
		}},

		GroupBox{Title: "Ethernet / TCP", Layout: Grid{Columns: 4, Margins: Margins{Left: 16, Top: 14, Right: 16, Bottom: 16}, Spacing: 10}, Children: []Widget{
			Label{Text: "IP de la balanza"}, LineEdit{AssignTo: &f.tcpHost, Text: cfg.TCP.Host},
			Label{Text: "Puerto TCP"}, LineEdit{AssignTo: &f.tcpPort, Text: strconv.Itoa(cfg.TCP.Port)},
			Label{Text: "Timeout (ms)"}, LineEdit{AssignTo: &f.tcpTimeout, Text: strconv.Itoa(cfg.TCP.ReadTimeoutMs)},
			Label{Text: ""},
		}},

		GroupBox{Title: "Interpretación de la lectura", Layout: Grid{Columns: 2, Margins: Margins{Left: 16, Top: 14, Right: 16, Bottom: 16}, Spacing: 10}, Children: []Widget{
			Label{Text: "Modelo"}, ComboBox{AssignTo: &f.scaleModel, Model: models, CurrentIndex: scaleModelIndex(cfg.Parser.Model),
				OnCurrentIndexChanged: func() {
					pat, unit := defaultPatternForModelIndex(f.scaleModel.CurrentIndex())
					f.pattern.SetText(pat)
					f.unit.SetText(unit)
				},
			},
			Label{Text: "Patrón (grupo weight)"}, LineEdit{AssignTo: &f.pattern, Text: cfg.Parser.Pattern},
			Label{Text: "Unidad predeterminada"}, LineEdit{AssignTo: &f.unit, Text: cfg.Parser.DefaultUnit},
		}},

		GroupBox{Title: "Impresora POS", Layout: Grid{Columns: 4, Margins: Margins{Left: 16, Top: 14, Right: 16, Bottom: 16}, Spacing: 10}, Children: []Widget{
			Label{Text: "Puerto COM"}, LineEdit{AssignTo: &f.printerPort, Text: cfg.Printer.Port},
			Label{Text: "Velocidad"}, LineEdit{AssignTo: &f.printerBaud, Text: strconv.Itoa(cfg.Printer.BaudRate)},
			Label{Text: "Bits de datos"}, LineEdit{AssignTo: &f.printerData, Text: strconv.Itoa(cfg.Printer.DataBits)},
			Label{Text: "Paridad"}, ComboBox{AssignTo: &f.printerParity, Model: parities, CurrentIndex: parityIndex(cfg.Printer.Parity)},
			Label{Text: "Bits de parada"}, LineEdit{AssignTo: &f.printerStop, Text: strconv.Itoa(cfg.Printer.StopBits)},
			Label{Text: "Nombre de empresa"}, LineEdit{AssignTo: &f.company, Text: cfg.Printer.Company},
			Label{Text: "RUC"}, LineEdit{AssignTo: &f.ruc, Text: cfg.Printer.RUC},
			Label{Text: "Actividad"}, LineEdit{AssignTo: &f.activity, Text: cfg.Printer.Activity},
			Label{Text: "Operador"}, LineEdit{AssignTo: &f.operator, Text: cfg.Printer.Operator},
			Label{Text: "Título"}, LineEdit{AssignTo: &f.title, Text: cfg.Printer.Title},
			Label{Text: "Pie del ticket"}, LineEdit{AssignTo: &f.footer, Text: cfg.Printer.Footer},
		}},
		Composite{Layout: HBox{}, Children: []Widget{PushButton{Text: "Detectar puertos COM", MinSize: Size{Width: 170, Height: 42}, OnClicked: detect}, HSpacer{}, PushButton{Text: "Guardar configuración", Font: Font{Family: "Segoe UI Semibold", PointSize: 10}, MinSize: Size{Width: 190, Height: 42}, OnClicked: save}}},
	}
}

func integrationWidgets(cfg *config.Config, f *fields, save func()) []Widget {
	return []Widget{
		Label{Text: "INTEGRACIÓN WEB", TextColor: walk.RGB(30, 41, 59), Font: Font{Family: "Segoe UI Semibold", PointSize: 16}},
		Label{Text: "Configura el acceso de aplicaciones web a la API local de la balanza.", TextColor: walk.RGB(71, 85, 105)},
		GroupBox{Title: "API local", MinSize: Size{Height: 210}, MaxSize: Size{Height: 210}, Layout: Grid{Columns: 2, Margins: Margins{Left: 18, Top: 16, Right: 18, Bottom: 18}, Spacing: 12}, Children: []Widget{
			Label{Text: "API local activa"}, CheckBox{AssignTo: &f.apiEnabled, Checked: cfg.API.Enabled},
			Label{Text: "Dirección"}, LineEdit{AssignTo: &f.apiAddress, Text: cfg.API.Address},
			Label{Text: "Orígenes permitidos"}, LineEdit{AssignTo: &f.apiOrigins, Text: strings.Join(cfg.API.AllowedOrigins, ", ")},
			Label{Text: "Formato"}, Label{Text: "Separe varios orígenes con comas. Ejemplo: http://127.0.0.1:5173"},
			Label{Text: "Clave de acceso (X-Api-Key)"}, Composite{Layout: HBox{MarginsZero: true, Spacing: 8}, Children: []Widget{
				LineEdit{AssignTo: &f.apiKey, Text: cfg.API.APIKey, PasswordMode: true},
				PushButton{Text: "Generar", MinSize: Size{Width: 80, Height: 30}, OnClicked: func() {
					b := make([]byte, 24)
					if _, err := rand.Read(b); err != nil {
						return
					}
					f.apiKey.SetText(hex.EncodeToString(b))
				}},
			}},
		}},
		Label{Text: "Deja la clave vacía para acceso sin restricción. Con clave, las peticiones deben incluir el header X-Api-Key.", TextColor: walk.RGB(100, 116, 139)},
		Composite{MinSize: Size{Height: 42}, MaxSize: Size{Height: 42}, Layout: HBox{}, Children: []Widget{HSpacer{}, PushButton{Text: "Guardar integración", Font: Font{Family: "Segoe UI Semibold", PointSize: 10}, MinSize: Size{Width: 190, Height: 42}, OnClicked: save}}},
		VSpacer{},
	}
}

func connTypeIndex(t string) int {
	switch t {
	case config.ConnectionUSB:
		return 1
	case config.ConnectionTCP:
		return 2
	}
	return 0
}

func connTypeFromIndex(idx int) string {
	switch idx {
	case 1:
		return config.ConnectionUSB
	case 2:
		return config.ConnectionTCP
	}
	return config.ConnectionSerial
}

func parityIndex(value string) int {
	if strings.EqualFold(value, "E") {
		return 1
	}
	if strings.EqualFold(value, "O") {
		return 2
	}
	return 0
}

func defaultPatternForModelIndex(idx int) (pattern, unit string) {
	switch idx {
	case 1: // Lexus Matrix Zero
		return `(?P<stable>=)\s*(?P<weight>\d+\.\d+)`, "kg"
	case 2: // CAS CL5200N
		return `(?P<stable>ST)\s+(?P<weight>\d+\.\d+)(?P<unit>kg|lb)`, "kg"
	default: // Genérica / CAS-Digi
		return `(?P<stable>=)\s*(?P<weight>\d+\.\d+)`, "kg"
	}
}

func scaleModelIndex(value string) int {
	switch value {
	case config.ScaleModelLexusMatrixZero:
		return 1
	case config.ScaleModelCASCL5200N:
		return 2
	}
	return 0
}

func saveForm(path string, cfg *config.Config, f fields) error {
	parseInt := func(name string, field *walk.LineEdit, min int) (int, error) {
		value, err := strconv.Atoi(strings.TrimSpace(field.Text()))
		if err != nil || value < min {
			return 0, fmt.Errorf("%s debe ser un número válido", name)
		}
		return value, nil
	}
	var err error

	// --- tipo de conexión ---
	cfg.Connection.Type = connTypeFromIndex(f.connType.CurrentIndex())

	// --- RS-232 serial ---
	if cfg.Serial.BaudRate, err = parseInt("Velocidad serial", f.scaleBaud, 1); err != nil {
		return err
	}
	if cfg.Serial.DataBits, err = parseInt("Bits de datos serial", f.scaleData, 5); err != nil {
		return err
	}
	if cfg.Serial.StopBits, err = parseInt("Bits de parada serial", f.scaleStop, 1); err != nil {
		return err
	}
	if cfg.Serial.ReadTimeoutMs, err = parseInt("Timeout serial", f.timeout, 1); err != nil {
		return err
	}
	cfg.Serial.Port = strings.TrimSpace(f.scalePort.Text())
	cfg.Serial.Parity = f.scaleParity.Text()

	// --- USB-CDC ---
	if cfg.USB.BaudRate, err = parseInt("Velocidad USB", f.usbBaud, 1); err != nil {
		return err
	}
	if cfg.USB.DataBits, err = parseInt("Bits de datos USB", f.usbData, 5); err != nil {
		return err
	}
	if cfg.USB.StopBits, err = parseInt("Bits de parada USB", f.usbStop, 1); err != nil {
		return err
	}
	if cfg.USB.ReadTimeoutMs, err = parseInt("Timeout USB", f.usbTimeout, 1); err != nil {
		return err
	}
	cfg.USB.Port = strings.TrimSpace(f.usbPort.Text())
	cfg.USB.Parity = f.usbParity.Text()

	// --- TCP Ethernet ---
	if cfg.TCP.Port, err = parseInt("Puerto TCP", f.tcpPort, 1); err != nil {
		return err
	}
	if cfg.TCP.ReadTimeoutMs, err = parseInt("Timeout TCP", f.tcpTimeout, 1); err != nil {
		return err
	}
	cfg.TCP.Host = strings.TrimSpace(f.tcpHost.Text())

	// validar campos obligatorios según tipo activo
	switch cfg.Connection.Type {
	case config.ConnectionSerial:
		if cfg.Serial.Port == "" {
			return fmt.Errorf("el puerto COM de la balanza es obligatorio")
		}
	case config.ConnectionUSB:
		if cfg.USB.Port == "" {
			return fmt.Errorf("el puerto COM USB es obligatorio")
		}
	case config.ConnectionTCP:
		if cfg.TCP.Host == "" {
			return fmt.Errorf("la IP de la balanza es obligatoria")
		}
	}

	// --- parser ---
	model := config.ScaleModelGeneric
	switch f.scaleModel.CurrentIndex() {
	case 1:
		model = config.ScaleModelLexusMatrixZero
	case 2:
		model = config.ScaleModelCASCL5200N
	}
	if _, err := serialreader.NewReader(&config.Config{Parser: config.ParserConfig{Model: model, Pattern: f.pattern.Text(), DefaultUnit: f.unit.Text()}}); err != nil {
		return fmt.Errorf("patrón de lectura inválido: %w", err)
	}
	cfg.Parser.Model = model
	cfg.Parser.Pattern = f.pattern.Text()
	cfg.Parser.DefaultUnit = strings.TrimSpace(f.unit.Text())

	// --- impresora ---
	if cfg.Printer.BaudRate, err = parseInt("Velocidad de impresora", f.printerBaud, 1); err != nil {
		return err
	}
	if cfg.Printer.DataBits, err = parseInt("Bits de datos de impresora", f.printerData, 5); err != nil {
		return err
	}
	if cfg.Printer.StopBits, err = parseInt("Bits de parada de impresora", f.printerStop, 1); err != nil {
		return err
	}
	cfg.Printer.Port = strings.TrimSpace(f.printerPort.Text())
	cfg.Printer.Parity = f.printerParity.Text()
	cfg.Printer.Company = strings.TrimSpace(f.company.Text())
	cfg.Printer.RUC = strings.TrimSpace(f.ruc.Text())
	cfg.Printer.Activity = strings.TrimSpace(f.activity.Text())
	cfg.Printer.Operator = strings.TrimSpace(f.operator.Text())
	cfg.Printer.Title = strings.TrimSpace(f.title.Text())
	cfg.Printer.Footer = strings.TrimSpace(f.footer.Text())

	// --- API ---
	cfg.API.Enabled = f.apiEnabled.Checked()
	cfg.API.Address = strings.TrimSpace(f.apiAddress.Text())
	if cfg.API.Enabled && cfg.API.Address == "" {
		return fmt.Errorf("la dirección de la API es obligatoria")
	}
	cfg.API.AllowedOrigins = nil
	for _, origin := range strings.Split(f.apiOrigins.Text(), ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			cfg.API.AllowedOrigins = append(cfg.API.AllowedOrigins, origin)
		}
	}
	cfg.API.APIKey = strings.TrimSpace(f.apiKey.Text())
	return config.Save(path, cfg)
}

func showPorts(owner walk.Form) {
	ports, err := seriallib.GetPortsList()
	if err != nil {
		walk.MsgBox(owner, "Puertos", err.Error(), walk.MsgBoxIconError)
		return
	}
	if len(ports) == 0 {
		walk.MsgBox(owner, "Puertos", "No se detectaron puertos seriales.", walk.MsgBoxIconWarning)
		return
	}
	walk.MsgBox(owner, "Puertos detectados", strings.Join(ports, "\r\n"), walk.MsgBoxIconInformation)
}
