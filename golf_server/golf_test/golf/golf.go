package golf

import (
	"syscall/js"
)

// Addresses
// ScreenBuff: 0x0000 - 0x3601
//  Col Buff: 0x0000 - 0x2400
// 	Pal Buff: 0x2401 - 0x3600
//  Pal Set: 0x3601
// BG Color: 0x3602 - high 3 bits
// CameraX: 0x3603-0x3604
// CameraY: 0x3605-0x3606
// Frames: 0x3607-0x3609
// ClipX: 0x360A
// ClipY: 0x360B
// ClipW: 0x360C
// ClipH: 0x360D
// Mouse:
//  X: 0x360E
//	Y: 0x360F
//	Left Click: 0x3610
//	Middle Click: 0x3610
//	Right Click: 0x3610
//	Mouse Style: 0x3610
// Keyboard: 0x3611-0x3647
// InternalSpriteSheet: 0x3648-0x3F48 [0x0900]
// SpriteSheet: 0x3F49-0x6F49 [0x3000]

// Engine Screen Width and Height
const (
	ScreenHeight = 192
	ScreenWidth  = 192
)

// Engine is the golf engine
type Engine struct {
	RAM           *[0xFFFF]byte
	screenBufHook js.Value
	Draw          func()
	Update        func()
}

// NewEngine creates a new golf engine
func NewEngine(updateFunc func(), draw func()) *Engine {
	ret := Engine{
		RAM:           &[0xFFFF]byte{},
		Draw:          draw,
		Update:        updateFunc,
		screenBufHook: js.Global().Get("screenBuff"),
	}

	ret.initKeyListener(js.Global().Get("document"))
	ret.initMouseListener(js.Global().Get("golfcanvas"))

	ret.RClip() // Reset the cliping box

	// Set internal resources
	base := 0x3648
	for i := 0; i < 0x0900; i++ {
		ret.RAM[i+base] = internalSprites[i]
	}

	//TODO inject the custom javascritp into the page here
	return &ret
}

// Run starts the game engine running
func (e *Engine) Run() {
	var renderFrame js.Func

	renderFrame = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		e.addFrame()

		e.Update()
		e.Draw()
		e.tickKeyboard()
		e.tickMouse()

		js.CopyBytesToJS(e.screenBufHook, e.RAM[:0x3601])
		js.Global().Call("drawScreen")
		js.Global().Call("requestAnimationFrame", renderFrame)

		return nil
	})

	done := make(chan struct{}, 0)

	js.Global().Call("requestAnimationFrame", renderFrame)
	<-done
}

// Frames is the number of frames since the engine was started
func (e *Engine) Frames() int {
	return toInt(e.RAM[0x3607:0x360A])
}

func (e *Engine) addFrame() {
	f := toInt(e.RAM[0x3607:0x360A])
	f++
	b := toBytes(f, 3)
	e.RAM[0x3607] = b[0]
	e.RAM[0x3608] = b[1]
	e.RAM[0x3609] = b[2]
}

// Mouse returns the X, Y coords of the mouse
func (e *Engine) Mouse() (int, int) {
	return toInt(e.RAM[0x360E:0x360F]), toInt(e.RAM[0x360F:0x3610])
}

// BG sets the bg color of the engine
func (e *Engine) BG(col Col) {
	e.RAM[0x3602] &= 0b00011111
	e.RAM[0x3602] |= byte(col << 5)
}

// Cls clears the screen and resets TextL and TextR
func (e *Engine) Cls() {
	c := e.RAM[0x3602] >> 5
	colBG := (c << 6) | (c << 4) | (c << 2) | c
	palBG := byte(0)
	if e.RAM[0x3602]>>7 == 1 {
		palBG = 0b11111111
	}

	for i := 0; i < 0x2400; i++ {
		e.RAM[i] = colBG
	}
	for i := 0x2401; i < 0x3600; i++ {
		e.RAM[i] = palBG
	}
}

// Camera moves the camera which modifies all draw functions
func (e *Engine) Camera(x, y int) {
	xb := toBytes(x, 2)
	yb := toBytes(y, 2)
	e.RAM[0x3603] = xb[0]
	e.RAM[0x3604] = xb[1]
	e.RAM[0x3605] = yb[0]
	e.RAM[0x3606] = yb[1]
}

// Rect draws a rectangle border on the screen
func (e *Engine) Rect(x, y, w, h int, col Col) {
	x -= toInt(e.RAM[0x3603:0x3605])
	y -= toInt(e.RAM[0x3605:0x3607])
	for r := 0; r < w; r++ {
		e.Pset(x+r, y, col)
		e.Pset(x+r, y+(h-1), col)
	}
	for c := 0; c < h; c++ {
		e.Pset(x, y+c, col)
		e.Pset(x+(w-1), y+c, col)
	}
}

// RectFill draws a filled rectangle one the screen
func (e *Engine) RectFill(x, y, w, h int, col Col) {
	x -= toInt(e.RAM[0x3603:0x3605])
	y -= toInt(e.RAM[0x3605:0x3607])
	for r := 0; r < w; r++ {
		for c := 0; c < h; c++ {
			e.Pset(r+x, c+y, col)
		}
	}
}

// Line draws a colored line
func (e *Engine) Line(x1, y1, x2, y2 int, col Col) {
	x1 -= toInt(e.RAM[0x3603:0x3605])
	x2 -= toInt(e.RAM[0x3603:0x3605])
	y1 -= toInt(e.RAM[0x3605:0x3607])
	y2 -= toInt(e.RAM[0x3605:0x3607])
	if x2 < x1 {
		x2, x1 = x1, x2
	}
	w := x2 - x1
	dh := (float64(y2) - float64(y1)) / float64(w)
	if w > 0 {
		for x := x1; x < x2; x++ {
			e.Pset(x, y1+int(dh*float64(x-x1)), col)
		}
		return
	}
	if y2 < y1 {
		y2, y1 = y1, y2
	}
	h := y2 - y1
	dw := (float64(x2) - float64(x1)) / float64(h)
	if h > 0 {
		for y := y1; y < y2; y++ {
			e.Pset(x1+int(dw*float64(y-y1)), y, col)
		}
	}
}

// Clip clips all functions that draw to the screen
func (e *Engine) Clip(x, y, w, h int) {
	e.RAM[0x360A] = byte(x)
	e.RAM[0x360B] = byte(y)
	e.RAM[0x360C] = byte(w)
	e.RAM[0x360D] = byte(h)
}

// RClip resets the screen cliping
func (e *Engine) RClip() {
	e.RAM[0x360A] = 0
	e.RAM[0x360B] = 0
	e.RAM[0x360C] = 192
	e.RAM[0x360D] = 192
}

// Pset sets a pixel on the screen
func (e *Engine) Pset(x, y int, col Col) {
	cshift := x % 4
	pshift := x % 8
	i := (x / 4) + y*48
	j := (x / 8) + y*24
	pixel := e.RAM[i]

	masks := []byte{
		0b11111100,
		0b11110011,
		0b11001111,
		0b00111111,
	}

	newCol := byte(col&0b00000011) << (cshift * 2)
	newPix := (pixel & masks[cshift]) | newCol
	e.RAM[i] = newPix

	newPal := byte((col&0b00000100)>>2) << pshift
	e.RAM[j+0x2401] &= (0b00000001 << pshift)
	e.RAM[j+0x2401] |= newPal
}

// Pget gets the color of a pixel on the screen
func (e *Engine) Pget(x, y int) Col {
	cshift := x % 4
	pshift := x % 8
	i := (x / 4) + y*48
	j := (x / 8) + y*24
	pixel := e.RAM[i]
	pal := (e.RAM[j+0x2401] >> pshift) & 0b00000001

	masks := []byte{
		0b00000011,
		0b00001100,
		0b00110000,
		0b11000000,
	}
	pixel &= masks[cshift]
	pixel >>= (cshift * 2)

	return Col(pixel & (pal << 2))
}

// LoadSprs loads the sprite sheet into memory
func (e *Engine) LoadSprs(sheet [0x3000]byte) {
	base := 0x3F49
	for i, b := range sheet {
		e.RAM[i+base] = b
	}
}

// SprOpts additional options for drawing sprites
type SprOpts struct {
	FlipH         bool
	FlipV         bool
	Transparent   Col
	PalFrom       []Col
	PalTo         []Col
	Width, Height int
}

// Spr draws 8x8 sprite n from the sprite sheet to the
// screen at x, y.
func (e *Engine) Spr(n, x, y int, opts ...SprOpts) {}

// SSpr draw a rect from the sprite sheet to the screen
// sx, sy, sw, and sh define the rect on the sprite sheet
// dx, dy is the location to draw on the screen
func (e *Engine) SSpr(sx, sy, sw, sh, dx, dy int, opts ...SprOpts) {
	//TODO: implement this
}

// subPixels is used to swap pixels based on a pallet swap
func subPixels(palFrom, palTo []Col, col Col) Col {
	if len(palFrom) == 0 {
		return col
	}
	for i, p := range palFrom {
		if p == col {
			return palTo[i]
		}
	}
	return col
}

// TextOpts additional options for drawing text
type TextOpts struct {
	Transparent Col
	Col         Col
	Relative    bool
}

// TextL prints text at the top left of the screen
// the cursor moves to a new line each time TextL is called
func (e *Engine) TextL(text string, opts ...TextOpts) {}

// TextR prints text at the top right of the screen
// the cursor moves to a new line each time TextR is called
func (e *Engine) TextR(text string, opts ...TextOpts) {}

// Text prints text at the x, y coords on the screen
func (e *Engine) Text(text string, x, y int, opts ...TextOpts) {}

// PalA sets pallet A
func (e *Engine) PalA(pallet Pal) {
	e.RAM[0x3601] &= 0b00001111
	e.RAM[0x3601] |= byte(pallet << 4)
}

// PalB sets pallet B
func (e *Engine) PalB(pallet Pal) {
	e.RAM[0x3601] &= 0b11110000
	e.RAM[0x3601] |= byte(pallet)
}

// PalGet gets the currently set pallets
func (e *Engine) PalGet() (Pal, Pal) {
	return Pal(e.RAM[0x3601] >> 4), Pal(e.RAM[0x3601] & 0b00001111)
}

// Col is a screen color
type Col byte

// These are the pallet and color constants
const (
	Col0 = Col(0b10000000)
	Col1 = Col(0b10000001)
	Col2 = Col(0b10000010)
	Col3 = Col(0b10000011)
	Col4 = Col(0b10000100)
	Col5 = Col(0b10000101)
	Col6 = Col(0b10000110)
	Col7 = Col(0b10000111)
)

// Pal is a screen pallet
type Pal byte

// The list of all pallets
const (
	Pal0  = Pal(0b00000000)
	Pal1  = Pal(0b00000001)
	Pal2  = Pal(0b00000010)
	Pal3  = Pal(0b00000011)
	Pal4  = Pal(0b00000100)
	Pal5  = Pal(0b00000101)
	Pal6  = Pal(0b00000110)
	Pal7  = Pal(0b00000111)
	Pal8  = Pal(0b00001000)
	Pal9  = Pal(0b00001001)
	Pal10 = Pal(0b00001010)
	Pal11 = Pal(0b00001011)
	Pal12 = Pal(0b00001100)
	Pal13 = Pal(0b00001101)
	Pal14 = Pal(0b00001110)
	Pal15 = Pal(0b00001111)
)

func toInt(b []byte) int {
	ret := []byte{0, 0, 0, 0}
	l := len(b)
	for i := 0; i < 4; i++ {
		if l-i-1 > -1 {
			ret[3-i] = b[l-i-1]
		}
	}
	return int(ret[0])<<24 | int(ret[1])<<16 | int(ret[2])<<8 | int(ret[3])
}

func toBytes(i int, l int) []byte {
	if l == 1 {
		return []byte{byte(i)}
	}
	if l == 2 {
		return []byte{byte(i >> 8), byte(i)}
	}
	if l == 3 {
		return []byte{byte(i >> 16), byte(i >> 8), byte(i)}
	}

	return []byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)}
}

var internalSprites = [2304]byte{0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b11000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000000}