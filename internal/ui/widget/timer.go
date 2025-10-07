package widget

import (
	"fmt"
	"time"

	"github.com/mokiat/gog/opt"
	"github.com/mokiat/gomath/sprec"
	"github.com/mokiat/lacking/ui"
	co "github.com/mokiat/lacking/ui/component"
	"github.com/mokiat/lacking/ui/layout"
	"github.com/mokiat/lacking/ui/std"
)

type TimeProvider interface {
	ElapsedTime() time.Duration
}

var Timer = co.Define(&timerComponent{})

type TimerData struct {
	Provider TimeProvider
}

type timerComponent struct {
	co.BaseComponent

	provider TimeProvider

	bgImage *ui.Image
	font    *ui.Font
}

func (c *timerComponent) OnCreate() {
	data := co.GetData[TimerData](c.Properties())
	c.provider = data.Provider

	c.bgImage = co.OpenImage(c.Scope(), "ui/images/lower-right.png")
	c.font = co.OpenFont(c.Scope(), "ui:///roboto-bold.ttf")
}

func (c *timerComponent) Render() co.Instance {
	return co.New(std.Element, func() {
		co.WithLayoutData(c.Properties().LayoutData())
		co.WithData(std.ElementData{
			Essence:   c,
			Layout:    layout.Anchor(),
			IdealSize: opt.V(ui.NewSize(240, 48)),
		})
	})
}

func (c *timerComponent) OnRender(element *ui.Element, canvas *ui.Canvas) {
	drawBounds := canvas.DrawBounds(element, false)
	canvas.Reset()
	canvas.Rectangle(
		drawBounds.Position,
		drawBounds.Size,
	)
	/*
		canvas.Fill(ui.Fill{
			Color:       ui.White(),
			Image:       c.bgImage,
			ImageOffset: drawBounds.Position,
			ImageSize:   drawBounds.Size,
		})
	*/

	elapsedTime := c.provider.ElapsedTime()
	minutes := int(elapsedTime.Seconds()) / 60
	seconds := int(elapsedTime.Seconds()) % 60
	millis := int(elapsedTime.Milliseconds()) % 1000

	text := fmt.Sprintf("%02d:%02d.%03d", minutes, seconds, millis)
	fontSize := float32(48.0)

	canvas.Reset()
	canvas.FillText(text, sprec.Vec2{
		X: (240 - c.font.TextSize(text, fontSize).X) / 2,
		Y: 0,
	}, ui.Typography{
		Font:  c.font,
		Size:  fontSize,
		Color: ui.White(),
	})

	element.Invalidate()
}
