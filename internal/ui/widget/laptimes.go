package widget

import (
	"fmt"
	"strings"
	"time"

	"github.com/mokiat/gog/opt"
	"github.com/mokiat/gomath/sprec"
	"github.com/mokiat/lacking/ui"
	co "github.com/mokiat/lacking/ui/component"
	"github.com/mokiat/lacking/ui/layout"
	"github.com/mokiat/lacking/ui/std"
)

type LapTimesProvider interface {
	LapTimes() []time.Duration
}

var LapTimes = co.Define(&lapTimesComponent{})

type LapTimesData struct {
	Provider LapTimesProvider
}

type lapTimesComponent struct {
	co.BaseComponent

	provider LapTimesProvider

	bgImage *ui.Image
	font    *ui.Font
}

func (c *lapTimesComponent) OnCreate() {
	data := co.GetData[LapTimesData](c.Properties())
	c.provider = data.Provider

	c.bgImage = co.OpenImage(c.Scope(), "ui/images/lower-right.png")
	c.font = co.OpenFont(c.Scope(), "ui:///roboto-bold.ttf")
}

func (c *lapTimesComponent) Render() co.Instance {
	return co.New(std.Element, func() {
		co.WithLayoutData(c.Properties().LayoutData())
		co.WithData(std.ElementData{
			Essence:   c,
			Layout:    layout.Anchor(),
			IdealSize: opt.V(ui.NewSize(240, 36*4)),
		})
	})
}

func (c *lapTimesComponent) OnRender(element *ui.Element, canvas *ui.Canvas) {
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

	lapTimes := c.provider.LapTimes()
	lines := []string{}
	for _, lapTime := range lapTimes[:4] {
		if lapTime == 0 {
			continue
		}
		minutes := int(lapTime.Seconds()) / 60
		seconds := int(lapTime.Seconds()) % 60
		millis := int(lapTime.Milliseconds()) % 1000
		lines = append(lines, fmt.Sprintf("%02d:%02d.%03d", minutes, seconds, millis))
	}
	text := strings.Join(lines, "\n")
	fontSize := float32(36.0)

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
