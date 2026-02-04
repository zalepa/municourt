package cmd

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"sort"
	"strings"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgpdf"
)

const (
	pageWidth  = 8.5 * vg.Inch
	pageHeight = 11 * vg.Inch
	pdfMargin  = 0.75 * vg.Inch
)

var chartBlue = color.RGBA{R: 31, G: 119, B: 180, A: 255}

func renderPDF(path, title string, series map[string][]dataPoint, sortedDates []string, includeStatewide bool, singleEntity bool) error {
	// Replace em dashes with plain dashes — the Liberation font in vgpdf
	// doesn't render the em dash glyph correctly.
	title = strings.ReplaceAll(title, "\u2014", "-")
	title = strings.ReplaceAll(title, "\u2013", "-")

	c := vgpdf.New(pageWidth, pageHeight)

	if singleEntity {
		var name string
		var points []dataPoint
		for k, v := range series {
			name = k
			points = v
			break
		}
		drawChartPage(c, title+" - "+name, points, sortedDates)
	} else {
		names := sortedEntityNames(series)

		var statewidePoints []dataPoint
		if includeStatewide && len(names) > 1 {
			stateAgg := make(map[string]float64)
			for _, pts := range series {
				for _, p := range pts {
					stateAgg[p.date] += p.value
				}
			}
			for _, d := range sortedDates {
				if v, ok := stateAgg[d]; ok {
					statewidePoints = append(statewidePoints, dataPoint{date: d, value: v})
				}
			}
		}

		drawSummaryPages(c, title, series, names, sortedDates, statewidePoints)

		for _, name := range names {
			c.NextPage()
			drawChartPage(c, title+" - "+name, series[name], sortedDates)
		}
		if len(statewidePoints) > 0 {
			c.NextPage()
			drawChartPage(c, title+" - STATEWIDE", statewidePoints, sortedDates)
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := c.WriteTo(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func sortedEntityNames(series map[string][]dataPoint) []string {
	names := make([]string, 0, len(series))
	for k := range series {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

const (
	summaryRowHeight = 0.30 * vg.Inch
	nameColWidth     = 2.2 * vg.Inch
	valueColWidth    = 0.9 * vg.Inch
)

func drawSummaryPages(c *vgpdf.Canvas, title string, series map[string][]dataPoint, names []string, sortedDates []string, statewidePoints []dataPoint) {
	usableW := pageWidth - 2*pdfMargin
	usableH := pageHeight - 2*pdfMargin
	sparkColWidth := usableW - nameColWidth - valueColWidth

	headerHeight := 1.0 * vg.Inch
	availableForRows := usableH - headerHeight
	maxRowsPerPage := int(availableForRows / summaryRowHeight)

	dateRange := ""
	if len(sortedDates) > 0 {
		dateRange = fmt.Sprintf("%s to %s (%d periods)", sortedDates[0], sortedDates[len(sortedDates)-1], len(sortedDates))
	}

	type row struct {
		name   string
		points []dataPoint
		isSep  bool
	}

	var rows []row
	for _, n := range names {
		rows = append(rows, row{name: n, points: series[n]})
	}
	if len(statewidePoints) > 0 {
		rows = append(rows, row{isSep: true})
		rows = append(rows, row{name: "STATEWIDE", points: statewidePoints})
	}

	pageNum := 0
	rowIdx := 0
	for rowIdx < len(rows) {
		if pageNum > 0 {
			c.NextPage()
		}
		pageNum++

		dc := draw.New(c)
		area := draw.Crop(dc, pdfMargin, -pdfMargin, pdfMargin, -pdfMargin)

		var yTop vg.Length
		if pageNum == 1 {
			yTop = area.Max.Y
			fillText(area, title, vg.Points(14), area.Min.X, yTop-vg.Points(14), color.Black)
			fillText(area, dateRange, vg.Points(10), area.Min.X, yTop-0.35*vg.Inch, color.Gray{Y: 100})

			headerY := yTop - 0.6*vg.Inch
			fillText(area, "Entity", vg.Points(10), area.Min.X, headerY, color.Gray{Y: 80})
			fillText(area, "Latest", vg.Points(10), area.Min.X+nameColWidth, headerY, color.Gray{Y: 80})
			fillText(area, "Trend", vg.Points(10), area.Min.X+nameColWidth+valueColWidth, headerY, color.Gray{Y: 80})

			sepY := headerY - vg.Points(6)
			strokeHLine(area, area.Min.X, area.Min.X+usableW, sepY, color.Gray{Y: 180})

			yTop = sepY - vg.Points(4)
		} else {
			yTop = area.Max.Y - vg.Points(8)
			fillText(area, title+" (continued)", vg.Points(10), area.Min.X, yTop, color.Gray{Y: 100})
			yTop -= 0.25 * vg.Inch
		}

		rowsThisPage := maxRowsPerPage
		if pageNum == 1 {
			rowsThisPage = int((yTop - area.Min.Y) / summaryRowHeight)
		}

		drawn := 0
		for rowIdx < len(rows) && drawn < rowsThisPage {
			r := rows[rowIdx]
			rowIdx++
			if r.isSep {
				y := yTop - vg.Length(drawn)*summaryRowHeight - vg.Points(4)
				strokeHLine(area, area.Min.X, area.Min.X+usableW, y, color.Gray{Y: 180})
				continue
			}
			y := yTop - vg.Length(drawn)*summaryRowHeight - summaryRowHeight*0.65
			fillText(area, r.name, vg.Points(9), area.Min.X, y, color.Black)

			vals := alignValues(r.points, sortedDates)
			latest := lastNonNaN(vals)
			fillText(area, formatNum(latest), vg.Points(9), area.Min.X+nameColWidth, y, color.Black)

			sparkX := area.Min.X + nameColWidth + valueColWidth
			sparkY := yTop - vg.Length(drawn)*summaryRowHeight - summaryRowHeight + vg.Points(2)
			sparkArea := draw.Canvas{
				Canvas: area.Canvas,
				Rectangle: vg.Rectangle{
					Min: vg.Point{X: sparkX, Y: sparkY},
					Max: vg.Point{X: sparkX + sparkColWidth, Y: sparkY + summaryRowHeight - vg.Points(3)},
				},
			}
			drawSparkline(sparkArea, vals)

			drawn++
		}
	}
}

func drawSparkline(c draw.Canvas, vals []float64) {
	var pts plotter.XYs
	for i, v := range vals {
		if !math.IsNaN(v) {
			pts = append(pts, plotter.XY{X: float64(i), Y: v})
		}
	}
	if len(pts) < 2 {
		return
	}

	p := plot.New()
	p.HideAxes()
	p.BackgroundColor = color.Transparent

	line, err := plotter.NewLine(pts)
	if err != nil {
		return
	}
	line.Color = chartBlue
	line.Width = vg.Points(1.5)
	p.Add(line)

	p.X.Min = 0
	p.X.Max = float64(len(vals) - 1)
	minY, maxY := pts[0].Y, pts[0].Y
	for _, pt := range pts {
		if pt.Y < minY {
			minY = pt.Y
		}
		if pt.Y > maxY {
			maxY = pt.Y
		}
	}
	pad := (maxY - minY) * 0.1
	if pad == 0 {
		pad = 1
	}
	p.Y.Min = minY - pad
	p.Y.Max = maxY + pad

	p.Draw(c)
}

func drawChartPage(c *vgpdf.Canvas, title string, points []dataPoint, sortedDates []string) {
	sort.Slice(points, func(i, j int) bool {
		return points[i].date < points[j].date
	})
	var filtered []dataPoint
	for _, p := range points {
		if !math.IsNaN(p.value) {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		return
	}

	dateIdx := make(map[string]int, len(sortedDates))
	for i, d := range sortedDates {
		dateIdx[d] = i
	}

	pts := make(plotter.XYs, len(filtered))
	for i, dp := range filtered {
		x, ok := dateIdx[dp.date]
		if !ok {
			x = i
		}
		pts[i] = plotter.XY{X: float64(x), Y: dp.value}
	}

	p := plot.New()
	p.Title.Text = title
	p.Title.TextStyle.Font.Size = vg.Points(12)
	p.BackgroundColor = color.White

	line, err := plotter.NewLine(pts)
	if err != nil {
		return
	}
	line.Color = chartBlue
	line.Width = vg.Points(2)

	scatter, err := plotter.NewScatter(pts)
	if err != nil {
		return
	}
	scatter.Color = chartBlue
	scatter.Radius = vg.Points(3)
	scatter.Shape = draw.CircleGlyph{}

	p.Add(line, scatter, plotter.NewGrid())

	p.X.Tick.Marker = dateTicks(sortedDates)
	p.X.Min = -0.5
	p.X.Max = float64(len(sortedDates)) - 0.5
	p.X.Tick.Label.Rotation = math.Pi / 4
	p.X.Tick.Label.XAlign = draw.XRight
	p.X.Tick.Label.YAlign = draw.YCenter

	p.Y.Tick.Marker = numTicks{}

	dc := draw.New(c)
	area := draw.Crop(dc, pdfMargin, -pdfMargin, pdfMargin, -pdfMargin)
	p.Draw(area)
}

type dateTicks []string

func (dt dateTicks) Ticks(min, max float64) []plot.Tick {
	var ticks []plot.Tick
	n := len(dt)
	if n == 0 {
		return ticks
	}

	step := 1
	if n > 12 {
		step = (n + 11) / 12
	}

	for i := 0; i < n; i++ {
		t := plot.Tick{Value: float64(i)}
		if i%step == 0 {
			t.Label = dt[i]
		}
		ticks = append(ticks, t)
	}
	return ticks
}

type numTicks struct{}

func (numTicks) Ticks(min, max float64) []plot.Tick {
	t := plot.DefaultTicks{}
	ticks := t.Ticks(min, max)
	for i := range ticks {
		if ticks[i].Label != "" {
			ticks[i].Label = formatCompact(ticks[i].Value)
		}
	}
	return ticks
}

func fillText(c draw.Canvas, txt string, size vg.Length, x, y vg.Length, clr color.Color) {
	sty := draw.TextStyle{
		Color:   clr,
		Font:    plot.DefaultFont,
		Handler: plot.DefaultTextHandler,
	}
	sty.Font.Size = size
	c.FillText(sty, vg.Point{X: x, Y: y}, txt)
}

func strokeHLine(c draw.Canvas, x0, x1, y vg.Length, clr color.Color) {
	c.StrokeLine2(draw.LineStyle{
		Color: clr,
		Width: vg.Points(0.5),
	}, x0, y, x1, y)
}
