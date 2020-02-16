package escpos

import (
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
	"strconv"
	"strings"

	"github.com/astaxie/beego/logs"
	"github.com/qiniu/iconv"
)

var beelog = logs.NewLogger(10000)

func init() {
	beelog.SetLogger("console", "")
	beelog.EnableFuncCallDepth(true)
}

// text replacement map
var textReplaceMap = map[string]string{
	// horizontal tab
	"&#9;":  "\x09",
	"&#x9;": "\x09",

	// linefeed
	"&#10;": "\n",
	"&#xA;": "\n",

	// xml stuff
	"&apos;": "'",
	"&quot;": `"`,
	"&gt;":   ">",
	"&lt;":   "<",

	// ampersand must be last to avoid double decoding
	"&amp;": "&",
}

// replace text from the above map
func textReplace(data string) string {
	for k, v := range textReplaceMap {
		data = strings.Replace(data, k, v, -1)
	}
	return data
}

// Escpos struct
type Escpos struct {
	// destination
	dst io.Writer

	// font metrics
	width, height uint8

	// state toggles ESC[char]
	underline  uint8
	emphasize  uint8
	upsidedown uint8
	rotate     uint8

	// state toggles GS[char]
	reverse uint8

	Verbose bool
}

// reset toggles
func (e *Escpos) reset() {
	e.width = 1
	e.height = 1

	e.underline = 0
	e.emphasize = 0
	e.upsidedown = 0
	e.rotate = 0

	e.reverse = 0
}

// New create a Escpos printer
func New(dst io.Writer) (e *Escpos) {
	e = &Escpos{dst: dst}
	e.reset()
	return
}

// WriteRaw write raw bytes to printer
func (e *Escpos) WriteRaw(data []byte) (n int, err error) {
	if len(data) > 0 {
		if e.Verbose {
			beelog.Debug("Writing %d bytes: %s\n", len(data), data)
		}
		e.dst.Write(data)
	} else {
		if e.Verbose {
			beelog.Debug("Wrote NO bytes\n")
		}
	}

	return 0, nil
}

// Write a string to the printer
func (e *Escpos) Write(data string) (int, error) {
	return e.WriteRaw([]byte(data))
}

// WriteGBK write a string to the printer with GBK encode
func (e *Escpos) WriteGBK(data string) (int, error) {
	cd, err := iconv.Open("gbk", "utf-8")
	if err != nil {
		beelog.Critical("iconv.Open failed!")
		return 0, err
	}
	defer cd.Close()
	gbk := cd.ConvString(data)
	return e.WriteRaw([]byte(gbk))
}

// Init printer settings
// \x1B@ => ESC @
func (e *Escpos) Init() {
	e.reset()
	e.Write("\x1B@")
}

// Cut the paper
// \x1DVA0 => GS V A 0
func (e *Escpos) Cut() {
	e.Write("\x1DVA0")
}

// BanFeedButton
// \x1Bc5n => ESC c 5 n  n= 0, 1
func (e *Escpos) BanFeedButton(n uint8) {
	s := string([]byte{'\x1B', 'c', '5', n})
	e.Write(s)
}

// Beep ...
// \x1BBnt => ESC B n t
func (e *Escpos) Beep(n uint8) {
	s := string([]byte{'\x1B', 'B', n, 9})
	e.Write(s)
}

// Linefeed ...
func (e *Escpos) Linefeed() {
	e.Write("\n")
}

// FormfeedD ...
// \x1BJn => ESC J n n*0.125mm 0<=n<=255
func (e *Escpos) FormfeedD(n uint8) {
	if n < 0 {
		n = 0
	} else if n > 255 {
		n = 255
	}
	s := string([]byte{'\x1B', 'J', n})
	e.Write(s)
}

// FormfeedN ...
// \x1Bdn => ESC d n 0<=n<=255
func (e *Escpos) FormfeedN(n uint8) {
	if n < 0 {
		n = 0
	} else if n > 255 {
		n = 255
	}
	s := string([]byte{'\x1B', 'J', n})
	e.Write(s)
}

// Formfeed ...
func (e *Escpos) Formfeed() {
	e.FormfeedN(1)
}

// SetFont ...
// \x1BMn => ESC M n A(12*24) B(9*17) C(don't know)
func (e *Escpos) SetFont(font string) {
	f := 0

	switch font {
	case "A":
		f = 0
	case "B":
		f = 1
	case "C":
		f = 2
	default:
		beelog.Warn(fmt.Sprintf("Invalid font: '%s', defaulting to 'A'", font))
		f = 0
	}

	e.Write(fmt.Sprintf("\x1BM%c", f))
}

func (e *Escpos) sendFontSize() {
	s := string([]byte{'\x1D', '!', ((e.width - 1) << 4) | (e.height - 1)})
	e.Write(s)
}

// SetFontSize ...
// \x1D!n => GS ! n
func (e *Escpos) SetFontSize(width, height uint8) {
	if width > 0 && height > 0 && width <= 8 && height <= 8 {
		if height > 5 {
			height = 5
			beelog.Warn("change height to 5, because height larger than 5 may cause some error")
		}
		e.width = width
		e.height = height
		e.sendFontSize()
	} else {
		beelog.Critical(fmt.Sprintf("Invalid font size passed: %d x %d", width, height))
	}
}

func (e *Escpos) sendUnderline() {
	s := string([]byte{'\x1B', '-', e.underline})
	e.Write(s)
}

func (e *Escpos) sendEmphasize() {
	s := string([]byte{'\x1B', 'E', e.emphasize})
	e.Write(s)
}

func (e *Escpos) sendUpsidedown() {
	s := string([]byte{'\x1B', '{', e.upsidedown})
	e.Write(s)
}

func (e *Escpos) sendRotate() {
	s := string([]byte{'\x1B', 'V', e.rotate})
	e.Write(s)
}

func (e *Escpos) sendReverse() {
	s := string([]byte{'\x1D', 'B', e.reverse})
	e.Write(s)
}

func (e *Escpos) sendMoveX(x uint16) {
	e.Write(string([]byte{0x1b, 0x24, byte(x % 256), byte(x / 256)}))
}

func (e *Escpos) sendMoveY(y uint16) {
	e.Write(string([]byte{0x1d, 0x24, byte(y % 256), byte(y / 256)}))
}

// SetUnderline ...
// \x1B-n => ESC - n
func (e *Escpos) SetUnderline(v uint8) {
	e.underline = v
	e.sendUnderline()
}

// SetEmphasize ...
// \x1BGn => ESC E n n = 0, 1
func (e *Escpos) SetEmphasize(u uint8) {
	e.emphasize = u
	e.sendEmphasize()
}

// SetUpsidedown ...
// \x1B{n => ESC { n n = 0, 1
func (e *Escpos) SetUpsidedown(v uint8) {
	e.upsidedown = v
	e.sendUpsidedown()
}

// SetRotate ...
// \x1BVn => ESC V n
func (e *Escpos) SetRotate(v uint8) {
	e.rotate = v
	e.sendRotate()
}

// SetReverse ...
// GS B n n = 0, 1
func (e *Escpos) SetReverse(v uint8) {
	e.reverse = v
	e.sendReverse()
}

// SetMoveX ...
// \x1B$nLnH => ESC $ nL nH
func (e *Escpos) SetMoveX(x uint16) {
	e.sendMoveX(x)
}

// Pulse (open the drawer)
func (e *Escpos) Pulse() {
	// with t=2 -- meaning 2*2msec
	e.Write("\x1Bp\x02")
}

// SetLineSpace ...
// \x1B3n => ESC 3 n n*0.125mm
func (e *Escpos) SetLineSpace(n ...uint8) {
	var s string
	switch len(n) {
	case 0:
		s = string([]byte{'\x1B', '2'})
	case 1:
		s = string([]byte{'\x1B', '3', n[0]})
	default:
		beelog.Warn("Invalid num of params, using first param")
		s = string([]byte{'\x1B', '3', n[0]})
	}
	e.Write(s)
}

// SetAlign ...
// \x1Ban => ESC a n
func (e *Escpos) SetAlign(align string) {
	a := 0
	switch align {
	case "left":
		a = 0
	case "center":
		a = 1
	case "right":
		a = 2
	default:
		beelog.Warn(fmt.Sprintf("Invalid alignment: %s", align))
	}
	e.Write(fmt.Sprintf("\x1Ba%c", a))
}

// Text ...
func (e *Escpos) Text(params map[string]string, data string) {

	// send alignment to printer
	if align, ok := params["Align"]; ok {
		e.SetAlign(align)
	}

	// set emphasize
	if em, ok := params["EM"]; ok && (em == "true" || em == "1") {
		e.SetEmphasize(1)
	}

	// set underline
	if ul, ok := params["UL"]; ok && (ul == "true" || ul == "1") {
		e.SetUnderline(1)
	}

	// set reverse
	if reverse, ok := params["Reverse"]; ok && (reverse == "true" || reverse == "1") {
		e.SetReverse(1)
	}

	// set rotate
	if rotate, ok := params["Rotate"]; ok && (rotate == "true" || rotate == "1") {
		e.SetRotate(1)
	}

	// set font
	if font, ok := params["Font"]; ok {
		e.SetFont(strings.ToUpper(font[5:6]))
	}

	// do dw (double font width)
	if dw, ok := params["DW"]; ok && (dw == "true" || dw == "1") {
		e.SetFontSize(2, e.height)
	}

	// do dh (double font height)
	if dh, ok := params["DH"]; ok && (dh == "true" || dh == "1") {
		e.SetFontSize(e.width, 2)
	}

	// do font width
	if width, ok := params["Width"]; ok {
		if i, err := strconv.Atoi(width); err == nil {
			e.SetFontSize(uint8(i), e.height)
		} else {
			beelog.Critical(fmt.Sprintf("Invalid font width: %s", width))
		}
	}

	// do font height
	if height, ok := params["Height"]; ok {
		if i, err := strconv.Atoi(height); err == nil {
			e.SetFontSize(e.width, uint8(i))
		} else {
			beelog.Critical(fmt.Sprintf("Invalid font height: %s", height))
		}
	}

	// do y positioning
	if x, ok := params["X"]; ok {
		if i, err := strconv.Atoi(x); err == nil {
			e.sendMoveX(uint16(i))
		} else {
			beelog.Critical("Invalid x param %d", x)
		}
	}

	// do y positioning
	if y, ok := params["Y"]; ok {
		if i, err := strconv.Atoi(y); err == nil {
			e.sendMoveY(uint16(i))
		} else {
			beelog.Critical("Invalid y param %d", y)
		}
	}

	// do text replace, then write data
	data = textReplace(data)
	if len(data) > 0 {
		e.Write(data)
	}
}

// Feed ...
func (e *Escpos) Feed(params map[string]string) {
	// handle lines (form feed X lines)
	if l, ok := params["Line"]; ok {
		if i, err := strconv.Atoi(l); err == nil {
			e.FormfeedN(uint8(i))
		} else {
			beelog.Critical(fmt.Sprintf("Invalid line number %s", l))
		}
	}

	// handle units (dots)
	if u, ok := params["Unit"]; ok {
		if i, err := strconv.Atoi(u); err == nil {
			e.sendMoveY(uint16(i))
		} else {
			beelog.Critical(fmt.Sprintf("Invalid unit number %s", u))
		}
	}

	// send linefeed
	e.Linefeed()

	// reset variables
	e.reset()

	// reset printer
	e.sendEmphasize()
	e.sendRotate()
	e.sendReverse()
	e.sendUnderline()
	e.sendUpsidedown()
	e.sendFontSize()
}

// FeedAndCut ...
func (e *Escpos) FeedAndCut(params map[string]string) {
	if t, ok := params["Type"]; ok && t == "feed" {
		e.Formfeed()
	}

	e.Cut()
}

// used to send graphics headers
func (e *Escpos) gSend(m byte, fn byte, data []byte) {
	l := len(data) + 2

	e.Write("\x1b(L")
	e.WriteRaw([]byte{byte(l % 256), byte(l / 256), m, fn})
	e.WriteRaw(data)
}

// Image write an image
func (e *Escpos) Image(params map[string]string, data string) {
	// send alignment to printer
	if align, ok := params["Align"]; ok {
		e.SetAlign(align)
	}

	// get width
	widthStr, ok := params["Width"]
	if !ok {
		beelog.Critical("No width specified on image")
	}

	// get height
	heightStr, ok := params["Height"]
	if !ok {
		beelog.Critical("No height specified on image")
	}

	// convert width
	width, err := strconv.Atoi(widthStr)
	if err != nil {
		beelog.Critical("Invalid image width %s", widthStr)
	}

	// convert height
	height, err := strconv.Atoi(heightStr)
	if err != nil {
		beelog.Critical("Invalid image height %s", heightStr)
	}

	if e.Verbose {
		beelog.Debug("Image len:%d w: %d h: %d\n", len(data), width, height)
	}

	e.PrintImage(data)
}

// WriteNode write a "node" to the printer
func (e *Escpos) WriteNode(params map[string]string, data string) {
	debugStr := ""
	if data != "" {
		str := data[:]
		if len(data) > 40 {
			str = fmt.Sprintf("%s ...", data[0:40])
		}
		debugStr = fmt.Sprintf(" => '%s'", str)
	}

	name, ok := params["Name"]
	if ok {
		if name == "" {
			name = "pulse"
		}
	}

	if e.Verbose {
		beelog.Debug("Write: %s => %+v%s\n", name, params, debugStr)
	}

	switch name {
	case "text":
		e.Text(params, data)
	case "feed":
		e.Feed(params)
	case "cut":
		e.FeedAndCut(params)
	case "pulse":
		e.Pulse()
	case "image":
		e.Image(params, data)
	}
}

// taken https://github.com/mugli/png2escpos
//
func closestNDivisibleBy8(n int) int {
	q := n / 8
	n1 := q * 8

	return n1
}

func (e *Escpos) PrintImage(imgFile string) error {
	image.RegisterFormat("png", "png", png.Decode, png.DecodeConfig)

	width, height, pixels, err := getPixels(imgFile)

	if err != nil {
		return err
	}

	removeTransparency(&pixels)
	makeGrayscale(&pixels)

	printWidth := closestNDivisibleBy8(width)
	printHeight := closestNDivisibleBy8(height)
	bytes, _ := rasterize(printWidth, printHeight, &pixels)

	imageHeader := []byte{0x1d, 0x76, 0x30, 0x00}
	imageData := []byte{}
	imageData = append(imageHeader,
		byte((width>>3)&0xff),
		byte(((width>>3)>>8)&0xff),
		byte(height&0xff),
		byte((height>>8)&0xff))
	imageData = append(imageData, bytes...)

	e.WriteRaw(imageData)
	return err
}

func makeGrayscale(pixels *[][]pixel) {
	height := len(*pixels)
	width := len((*pixels)[0])

	for y := 0; y < height; y++ {
		row := (*pixels)[y]
		for x := 0; x < width; x++ {
			pixel := row[x]

			luminance := (float64(pixel.R) * 0.299) + (float64(pixel.G) * 0.587) + (float64(pixel.B) * 0.114)
			var value int
			if luminance < 128 {
				value = 0
			} else {
				value = 255
			}

			pixel.R = value
			pixel.G = value
			pixel.B = value

			row[x] = pixel
		}
	}
}

func removeTransparency(pixels *[][]pixel) {
	height := len(*pixels)
	width := len((*pixels)[0])

	for y := 0; y < height; y++ {
		row := (*pixels)[y]
		for x := 0; x < width; x++ {
			pixel := row[x]

			alpha := pixel.A
			invAlpha := 255 - alpha

			pixel.R = (alpha*pixel.R + invAlpha*255) / 255
			pixel.G = (alpha*pixel.G + invAlpha*255) / 255
			pixel.B = (alpha*pixel.B + invAlpha*255) / 255
			pixel.A = 255

			row[x] = pixel
		}
	}
}

func rasterize(printWidth int, printHeight int, pixels *[][]pixel) ([]byte, error) {
	if printWidth%8 != 0 {
		return nil, fmt.Errorf("printWidth must be a multiple of 8")
	}

	if printHeight%8 != 0 {
		return nil, fmt.Errorf("printHeight must be a multiple of 8")
	}

	bytes := make([]byte, (printWidth*printHeight)>>3)

	for y := 0; y < printHeight; y++ {
		for x := 0; x < printWidth; x = x + 8 {
			i := y*(printWidth>>3) + (x >> 3)
			bytes[i] =
				byte((getPixelValue(x+0, y, pixels) << 7) |
					(getPixelValue(x+1, y, pixels) << 6) |
					(getPixelValue(x+2, y, pixels) << 5) |
					(getPixelValue(x+3, y, pixels) << 4) |
					(getPixelValue(x+4, y, pixels) << 3) |
					(getPixelValue(x+5, y, pixels) << 2) |
					(getPixelValue(x+6, y, pixels) << 1) |
					getPixelValue(x+7, y, pixels))
		}
	}

	return bytes, nil
}

func getPixelValue(x int, y int, pixels *[][]pixel) int {
	row := (*pixels)[y]
	pixel := row[x]

	if pixel.R > 0 {
		return 0
	}

	return 1
}

func rgbaToPixel(r uint32, g uint32, b uint32, a uint32) pixel {
	return pixel{int(r >> 8), int(g >> 8), int(b >> 8), int(a >> 8)}
}

type pixel struct {
	R int
	G int
	B int
	A int
}

func getPixels(imgFile string) (int, int, [][]pixel, error) {

	infile := base64.NewDecoder(base64.StdEncoding, strings.NewReader(imgFile))
	img, _, err := image.Decode(infile)

	if err != nil {
		return 0, 0, nil, err
	}

	bounds := img.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y

	var pixels [][]pixel
	for y := 0; y < height; y++ {
		var row []pixel
		for x := 0; x < width; x++ {
			row = append(row, rgbaToPixel(img.At(x, y).RGBA()))
		}
		pixels = append(pixels, row)
	}

	return width, height, pixels, nil
}
