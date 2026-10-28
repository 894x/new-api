package wecomaibot

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// 纯 Go 图表渲染引擎：sfnt 加载中文字体，二倍超采样后 CatmullRom 缩小出图。
// 画布布局坐标一律使用最终像素，绘制原语内部乘 chartScale 进入超采样空间。
// 输出规范（企微客户端展示）：宽固定 1000px，高 ≤ 562px（宽×9/16），
// 总像素约 56 万（≤150 万），PNG 约 60-120KB（<2MB）。

const (
	chartScale = 2
	chartW     = 1000.0
	chartH     = 562.0

	chartBorderInset  = 8.0
	chartBorderRadius = 16.0
	chartBorderWidth  = 2.0
	chartPlotLeft     = 72.0
	chartPlotRight    = chartW - 36.0
	chartPlotTop      = 116.0
	chartXLabelSpace  = 30.0
	chartLegendSpace  = 38.0
	chartXLabelThinAt = 56.0
)

// Tableau 10 精选色板：饱和度适中、深浅交替，适合亮色背景。
var chartPalette = []color.RGBA{
	{R: 0x4E, G: 0x79, B: 0xA7, A: 0xFF},
	{R: 0xF2, G: 0x8E, B: 0x2B, A: 0xFF},
	{R: 0x59, G: 0xA1, B: 0x4F, A: 0xFF},
	{R: 0xE1, G: 0x57, B: 0x59, A: 0xFF},
	{R: 0xB0, G: 0x7A, B: 0xA1, A: 0xFF},
	{R: 0x76, G: 0xB7, B: 0xB2, A: 0xFF},
	{R: 0xED, G: 0xC9, B: 0x48, A: 0xFF},
	{R: 0x9C, G: 0x75, B: 0x5F, A: 0xFF},
	{R: 0xBA, G: 0xB0, B: 0xAC, A: 0xFF},
	{R: 0xD3, G: 0x72, B: 0x95, A: 0xFF},
}

var (
	chartTextTitle  = color.RGBA{R: 0x2B, G: 0x2B, B: 0x2B, A: 0xFF}
	chartTextSub    = color.RGBA{R: 0x8A, G: 0x8A, B: 0x8A, A: 0xFF}
	chartTextAxis   = color.RGBA{R: 0x55, G: 0x55, B: 0x55, A: 0xFF}
	chartTextValue  = color.RGBA{R: 0x44, G: 0x44, B: 0x44, A: 0xFF}
	chartGridLine   = color.RGBA{R: 0xE6, G: 0xE6, B: 0xE6, A: 0xFF}
	chartAxisLine   = color.RGBA{R: 0xBB, G: 0xBB, B: 0xBB, A: 0xFF}
	chartBorderLine = color.RGBA{R: 0xD9, G: 0xD9, B: 0xD9, A: 0xFF}
	chartBackground = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
)

// ---- 中文字体加载 ----

var (
	chartFontOnce sync.Once
	chartFont     *sfnt.Font
	chartFontErr  error
)

// loadChartFont 优先取环境变量指定的字体文件，其后按容器/桌面常见路径探测；
// TTC 集合优先挑名字含 SC 的简体变体。
func loadChartFont() (*sfnt.Font, error) {
	chartFontOnce.Do(func() {
		candidates := []string{
			os.Getenv("WECOM_AIBOT_CJK_FONT"),
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
			`C:\Windows\Fonts\msyh.ttc`,
		}
		for _, path := range candidates {
			if path == "" {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if parsed, err := sfnt.Parse(data); err == nil {
				chartFont = parsed
				return
			}
			collection, err := sfnt.ParseCollection(data)
			if err != nil {
				continue
			}
			for i := 0; i < collection.NumFonts(); i++ {
				member, err := collection.Font(i)
				if err != nil {
					continue
				}
				if name, err := member.Name(nil, sfnt.NameIDFamily); err == nil && strings.Contains(name, "SC") {
					chartFont = member
					return
				}
			}
			if first, err := collection.Font(0); err == nil {
				chartFont = first
				return
			}
		}
		chartFontErr = errors.New("未找到可用的中文字体（安装 fonts-noto-cjk 或设置 WECOM_AIBOT_CJK_FONT）")
	})
	return chartFont, chartFontErr
}

// chartFontAvailable 供测试探测当前环境能否渲染。
func chartFontAvailable() bool {
	_, err := loadChartFont()
	return err == nil
}

// ---- 画布与绘制原语 ----

type chartCanvas struct {
	img   *image.RGBA
	font  *sfnt.Font
	width float64
}

func newChartCanvas(width, height float64) (*chartCanvas, error) {
	parsed, err := loadChartFont()
	if err != nil {
		return nil, err
	}
	bounds := image.Rect(0, 0, int(width*chartScale), int(height*chartScale))
	canvas := &chartCanvas{
		img:   image.NewRGBA(bounds),
		font:  parsed,
		width: width,
	}
	for iy := 0; iy < bounds.Dy(); iy++ {
		for ix := 0; ix < bounds.Dx(); ix++ {
			canvas.img.SetRGBA(ix, iy, chartBackground)
		}
	}
	return canvas, nil
}

// set 写入一个超采样像素，带 alpha 混合。
func (c *chartCanvas) set(ix, iy int, clr color.RGBA) {
	bounds := c.img.Bounds()
	if ix < bounds.Min.X || iy < bounds.Min.Y || ix >= bounds.Max.X || iy >= bounds.Max.Y {
		return
	}
	if clr.A == 0xFF {
		c.img.SetRGBA(ix, iy, clr)
		return
	}
	bg := c.img.RGBAAt(ix, iy)
	inv := int(0xFF - clr.A)
	c.img.SetRGBA(ix, iy, color.RGBA{
		R: uint8((int(clr.R)*int(clr.A) + int(bg.R)*inv) / 0xFF),
		G: uint8((int(clr.G)*int(clr.A) + int(bg.G)*inv) / 0xFF),
		B: uint8((int(clr.B)*int(clr.A) + int(bg.B)*inv) / 0xFF),
		A: 0xFF,
	})
}

func (c *chartCanvas) fillRect(x, y, w, h float64, clr color.RGBA) {
	x0, y0 := int(x*chartScale), int(y*chartScale)
	x1, y1 := int((x+w)*chartScale), int((y+h)*chartScale)
	for iy := y0; iy < y1; iy++ {
		for ix := x0; ix < x1; ix++ {
			c.set(ix, iy, clr)
		}
	}
}

func (c *chartCanvas) fillCircle(cx, cy, r float64, clr color.RGBA) {
	cxSS, cySS, rSS := cx*chartScale, cy*chartScale, r*chartScale
	for iy := int(cySS - rSS - 1); iy <= int(cySS+rSS+1); iy++ {
		for ix := int(cxSS - rSS - 1); ix <= int(cxSS+rSS+1); ix++ {
			if (float64(ix)-cxSS)*(float64(ix)-cxSS)+(float64(iy)-cySS)*(float64(iy)-cySS) <= rSS*rSS {
				c.set(ix, iy, clr)
			}
		}
	}
}

// fillPolygon 扫描线填充（点为超采样坐标，奇偶规则）。
func (c *chartCanvas) fillPolygon(pts []image.Point, clr color.RGBA) {
	if len(pts) < 3 {
		return
	}
	minY, maxY := pts[0].Y, pts[0].Y
	for _, pt := range pts[1:] {
		minY = min(minY, pt.Y)
		maxY = max(maxY, pt.Y)
	}
	type edge struct{ x0, y0, x1, y1 float64 }
	var edges []edge
	for i := 0; i < len(pts); i++ {
		a, b := pts[i], pts[(i+1)%len(pts)]
		if a.Y != b.Y {
			edges = append(edges, edge{float64(a.X), float64(a.Y), float64(b.X), float64(b.Y)})
		}
	}
	for iy := minY; iy <= maxY; iy++ {
		centerY := float64(iy) + 0.5
		var xs []float64
		for _, e := range edges {
			if (e.y0 <= centerY && e.y1 > centerY) || (e.y1 <= centerY && e.y0 > centerY) {
				xs = append(xs, e.x0+(centerY-e.y0)*(e.x1-e.x0)/(e.y1-e.y0))
			}
		}
		for i := 0; i+1 < len(xs); i += 2 {
			for ix := int(math.Ceil(xs[i] - 0.5)); ix <= int(xs[i+1]+0.5); ix++ {
				c.set(ix, iy, clr)
			}
		}
	}
}

// ssPoint 把最终像素坐标转成超采样坐标点。
func (c *chartCanvas) ssPoint(x, y float64) image.Point {
	return image.Point{X: int(x * chartScale), Y: int(y * chartScale)}
}

// thickLine 画带圆头的粗线段。
func (c *chartCanvas) thickLine(x1, y1, x2, y2, width float64, clr color.RGBA) {
	half := width / 2
	if x1 == x2 && y1 == y2 {
		c.fillCircle(x1, y1, half, clr)
		return
	}
	dx, dy := x2-x1, y2-y1
	length := math.Sqrt(dx*dx + dy*dy)
	nx, ny := dy/length, -dx/length
	c.fillPolygon([]image.Point{
		c.ssPoint(x1+nx*half, y1+ny*half),
		c.ssPoint(x2+nx*half, y2+ny*half),
		c.ssPoint(x2-nx*half, y2-ny*half),
		c.ssPoint(x1-nx*half, y1-ny*half),
	}, clr)
	c.fillCircle(x1, y1, half, clr)
	c.fillCircle(x2, y2, half, clr)
}

// donutWedge 画环形扇区：角度为屏幕坐标（y 向下，顺时针为正），单位弧度。
func (c *chartCanvas) donutWedge(cx, cy, rOut, rIn, a0, a1 float64, clr color.RGBA) {
	cxSS, cySS := cx*chartScale, cy*chartScale
	for iy := int((cy - rOut - 1) * chartScale); iy <= int((cy+rOut+1)*chartScale); iy++ {
		for ix := int((cx - rOut - 1) * chartScale); ix <= int((cx+rOut+1)*chartScale); ix++ {
			dx := float64(ix) - cxSS
			dy := float64(iy) - cySS
			dist2 := dx*dx + dy*dy
			if dist2 > rOut*rOut*chartScale*chartScale || dist2 < rIn*rIn*chartScale*chartScale {
				continue
			}
			rel := math.Atan2(dy, dx) - a0
			for rel < 0 {
				rel += 2 * math.Pi
			}
			for rel >= 2*math.Pi {
				rel -= 2 * math.Pi
			}
			if rel <= a1-a0 {
				c.set(ix, iy, clr)
			}
		}
	}
}

const (
	textAlignLeft   = -1
	textAlignCenter = 0
	textAlignRight  = 1
)

// text 以视觉垂直中心定位绘制文本；bold 用超采样空间 1px 重影模拟。
func (c *chartCanvas) text(str string, x, y, size float64, clr color.RGBA, align int, bold bool) {
	if str == "" {
		return
	}
	face, err := opentype.NewFace(c.font, &opentype.FaceOptions{Size: size * chartScale, DPI: 72})
	if err != nil {
		return
	}
	metrics := face.Metrics()
	ascent := float64(metrics.Ascent) / 64
	descent := float64(metrics.Descent) / 64
	width := stringAdvance(face, str) / chartScale
	dotX := x
	switch align {
	case textAlignCenter:
		dotX -= width / 2
	case textAlignRight:
		dotX -= width
	}
	drawer := &font.Drawer{Dst: c.img, Src: image.NewUniform(clr), Face: face}
	baseline := y + (ascent+descent)/2 - descent
	drawAt := func(offset float64) {
		drawer.Dot = fixed.Point26_6{
			X: fixed.Int26_6((dotX + offset/chartScale) * chartScale * 64),
			Y: fixed.Int26_6(baseline * chartScale * 64),
		}
		drawer.DrawString(str)
	}
	drawAt(0)
	if bold {
		drawAt(1)
	}
}

// textWidth 按最终像素坐标测量文本宽度。
func (c *chartCanvas) textWidth(str string, size float64) float64 {
	face, err := opentype.NewFace(c.font, &opentype.FaceOptions{Size: size * chartScale, DPI: 72})
	if err != nil {
		return 0
	}
	return stringAdvance(face, str) / chartScale
}

// fitLabel 把标签按实测宽度截断到省略号，保证不超过给定宽度。
func (c *chartCanvas) fitLabel(label string, size, maxWidth float64) string {
	if maxWidth < 12 || c.textWidth(label, size) <= maxWidth {
		return label
	}
	runes := []rune(label)
	for len(runes) > 0 {
		runes = runes[:len(runes)-1]
		if c.textWidth(string(runes)+"…", size) <= maxWidth {
			return string(runes) + "…"
		}
	}
	return "…"
}

// strokeRoundedRect 画圆角描边框：四段直线 + 四段 90° 环弧（复用 donutWedge）。
func (c *chartCanvas) strokeRoundedRect(x, y, w, h, radius, thickness float64, clr color.RGBA) {
	half := thickness / 2
	c.thickLine(x+radius, y, x+w-radius, y, thickness, clr)
	c.thickLine(x+radius, y+h, x+w-radius, y+h, thickness, clr)
	c.thickLine(x, y+radius, x, y+h-radius, thickness, clr)
	c.thickLine(x+w, y+radius, x+w, y+h-radius, thickness, clr)
	// 屏幕坐标（y 向下）下角度顺时针为正：0=右、π/2=下、π=左、3π/2=上
	c.donutWedge(x+radius, y+radius, radius+half, radius-half, math.Pi, 3*math.Pi/2, clr)
	c.donutWedge(x+w-radius, y+radius, radius+half, radius-half, -math.Pi/2, 0, clr)
	c.donutWedge(x+w-radius, y+h-radius, radius+half, radius-half, 0, math.Pi/2, clr)
	c.donutWedge(x+radius, y+h-radius, radius+half, radius-half, math.Pi/2, math.Pi, clr)
}

func stringAdvance(face font.Face, str string) float64 {
	return float64(font.MeasureString(face, str)) / 64
}

// png 超采样缩小并编码为 PNG。
func (c *chartCanvas) png() ([]byte, error) {
	bounds := c.img.Bounds()
	final := image.NewRGBA(image.Rect(0, 0, bounds.Dx()/chartScale, bounds.Dy()/chartScale))
	xdraw.CatmullRom.Scale(final, final.Bounds(), c.img, bounds, xdraw.Over, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, final); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---- 坐标轴与格式化 ----

// niceAxis 生成整洁刻度：返回下界、上界与步长。
func niceAxis(lo, hi float64) (min, max, step float64) {
	if hi <= lo {
		hi = lo + 1
	}
	rawStep := (hi - lo) / 4
	magnitude := math.Pow(10, math.Floor(math.Log10(rawStep)))
	norm := rawStep / magnitude
	var nice float64
	switch {
	case norm <= 1:
		nice = 1
	case norm <= 2:
		nice = 2
	case norm <= 2.5:
		nice = 2.5
	case norm <= 5:
		nice = 5
	default:
		nice = 10
	}
	step = nice * magnitude
	min = math.Floor(lo/step) * step
	max = math.Ceil(hi/step) * step
	if max <= min {
		max = min + step
	}
	return
}

// humanNumber 千分位缩写（k/M/B），用于轴刻度与数值标注。
func humanNumber(value float64) string {
	magnitude := math.Abs(value)
	switch {
	case magnitude >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", value/1_000_000_000)
	case magnitude >= 1_000_000:
		return fmt.Sprintf("%.1fM", value/1_000_000)
	case magnitude >= 1_000:
		return fmt.Sprintf("%.1fk", value/1_000)
	case magnitude == math.Trunc(magnitude):
		return fmt.Sprintf("%d", int64(value))
	default:
		return fmt.Sprintf("%.2f", value)
	}
}

// ---- 图表类型入口 ----

// renderChartPNG 把图表规格渲染为 PNG（中文字体缺失时返回可解释错误）。
func renderChartPNG(spec *chartSpec) ([]byte, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	canvas, err := newChartCanvas(chartW, chartH)
	if err != nil {
		return nil, err
	}
	canvas.text(spec.Title, canvas.width/2, 42, 26, chartTextTitle, textAlignCenter, true)
	if spec.Subtitle != "" {
		canvas.text(spec.Subtitle, canvas.width/2, 70, 16, chartTextSub, textAlignCenter, false)
	}
	switch spec.Kind {
	case "line":
		drawLineChart(canvas, spec)
	case "bar":
		drawBarChart(canvas, spec)
	case "grouped_bar":
		drawGroupedBarChart(canvas, spec)
	case "donut":
		drawDonutChart(canvas, spec)
	}
	// 圆角边框：聊天气泡中界定图片边界
	canvas.strokeRoundedRect(chartBorderInset, chartBorderInset,
		canvas.width-2*chartBorderInset, chartH-2*chartBorderInset,
		chartBorderRadius, chartBorderWidth, chartBorderLine)
	encoded, err := canvas.png()
	if err != nil {
		return nil, err
	}
	if len(encoded) > maxChartPNGByte {
		return nil, fmt.Errorf("chart image is too large (%d bytes)", len(encoded))
	}
	return encoded, nil
}

// seriesBounds 返回全部系列的数值下界/上界。
func seriesBounds(series []chartSeries) (lo, hi float64) {
	lo, hi = series[0].Data[0], series[0].Data[0]
	for _, entry := range series {
		for _, value := range entry.Data {
			lo = min(lo, value)
			hi = max(hi, value)
		}
	}
	return
}

// drawAxes 以（调用方已含余量的）数值范围画网格、y 刻度、底轴与 x 标签，
// 返回数值到绘图区像素的映射。x 标签按栏宽实测截断，栏宽过窄时自动抽稀，
// 避免相邻标签互相侵入造成文字重叠。
func (c *chartCanvas) drawAxes(spec *chartSpec, lo, hi, plotBottom float64) (xFor func(int) float64, yFor func(float64) float64) {
	plotLeft, plotRight := chartPlotLeft, c.width-36.0
	plotTop := chartPlotTop
	axisMin, axisMax, step := niceAxis(lo, hi)
	yFor = func(value float64) float64 {
		return plotTop + (axisMax-value)/(axisMax-axisMin)*(plotBottom-plotTop)
	}
	for tick := axisMin; tick <= axisMax+step/2; tick += step {
		y := yFor(tick)
		c.fillRect(plotLeft, y-0.5, plotRight-plotLeft, 1, chartGridLine)
		c.text(humanNumber(tick), plotLeft-10, y, 15, chartTextAxis, textAlignRight, false)
	}
	c.fillRect(plotLeft, plotBottom-0.5, plotRight-plotLeft, 1, chartAxisLine)
	if spec.YLabel != "" {
		c.text(spec.YLabel, plotLeft, plotTop-22, 14, chartTextSub, textAlignLeft, false)
	}
	n := len(spec.Labels)
	xFor = func(i int) float64 {
		if n == 1 {
			return (plotLeft + plotRight) / 2
		}
		return plotLeft + float64(i)*(plotRight-plotLeft)/float64(n-1)
	}
	slot := (plotRight - plotLeft) / float64(n)
	thinning := 1
	if slot < chartXLabelThinAt {
		thinning = int(math.Ceil(chartXLabelThinAt / slot))
	}
	labelMax := min(slot*0.92, 220)
	for i, label := range spec.Labels {
		if i%thinning != 0 && i != n-1 {
			continue
		}
		// 首末标签不允许超出画布边界
		fitted := c.fitLabel(label, 15, labelMax)
		drawX := xFor(i)
		maxX := c.width - chartBorderInset - 14
		if center := drawX + c.textWidth(fitted, 15)/2; center > maxX {
			drawX -= center - maxX
		}
		if left := drawX - c.textWidth(fitted, 15)/2; left < chartBorderInset+14 {
			drawX += chartBorderInset + 14 - left
		}
		c.text(fitted, drawX, plotBottom+28, 15, chartTextAxis, textAlignCenter, false)
	}
	return xFor, yFor
}

// drawLegend 在指定高度画底部居中图例（色块 + 系列名），系列名按剩余宽度截断。
func (c *chartCanvas) drawLegend(series []chartSeries, centerY float64) {
	const (
		chipSize  = 14.0
		gap       = 8.0
		itemGap   = 26.0
		labelSize = 15.0
	)
	fitted := make([]string, len(series))
	items := make([]float64, len(series))
	total := 0.0
	for i, entry := range series {
		fitted[i] = c.fitLabel(entry.Name, labelSize, 180)
		items[i] = chipSize + gap + c.textWidth(fitted[i], labelSize)
		total += items[i]
		if i < len(series)-1 {
			total += itemGap
		}
	}
	x := (c.width - total) / 2
	for i := range series {
		c.fillRect(x, centerY-chipSize/2, chipSize, chipSize, chartPalette[i%len(chartPalette)])
		x += chipSize + gap
		c.text(fitted[i], x, centerY, labelSize, chartTextAxis, textAlignLeft, false)
		x += items[i] - chipSize - gap + itemGap
	}
}

func drawLineChart(c *chartCanvas, spec *chartSpec) {
	lo, hi := seriesBounds(spec.Series)
	span := hi - lo
	if span == 0 {
		span = max(math.Abs(hi)*0.1, 1)
	}
	plotBottom := chartH - chartXLabelSpace - 6
	if len(spec.Series) > 1 {
		plotBottom -= chartLegendSpace
	}
	xFor, yFor := c.drawAxes(spec, lo-span*0.12, hi+span*0.18, plotBottom)
	for si, entry := range spec.Series {
		clr := chartPalette[si%len(chartPalette)]
		if len(spec.Series) == 1 {
			points := []image.Point{c.ssPoint(xFor(0), plotBottom)}
			for i, value := range entry.Data {
				points = append(points, c.ssPoint(xFor(i), yFor(value)))
			}
			points = append(points, c.ssPoint(xFor(len(entry.Data)-1), plotBottom))
			c.fillPolygon(points, color.RGBA{R: clr.R, G: clr.G, B: clr.B, A: 26})
		}
		for i := 0; i+1 < len(entry.Data); i++ {
			c.thickLine(xFor(i), yFor(entry.Data[i]), xFor(i+1), yFor(entry.Data[i+1]), 4, clr)
		}
		for i, value := range entry.Data {
			c.fillCircle(xFor(i), yFor(value), 4.8, clr)
			c.fillCircle(xFor(i), yFor(value), 2.8, chartBackground)
		}
	}
	if len(spec.Series) == 1 {
		data := spec.Series[0].Data
		clr := chartPalette[0]
		maxIndex, minIndex := 0, 0
		for i, value := range data {
			if value > data[maxIndex] {
				maxIndex = i
			}
			if value < data[minIndex] {
				minIndex = i
			}
		}
		c.text(humanNumber(data[maxIndex])+spec.Unit, xFor(maxIndex), yFor(data[maxIndex])-22, 15, clr, textAlignCenter, true)
		c.text(humanNumber(data[minIndex])+spec.Unit, xFor(minIndex), yFor(data[minIndex])+24, 14, chartTextSub, textAlignCenter, false)
	} else {
		c.drawLegend(spec.Series, chartH-chartLegendSpace+16)
	}
}

func drawBarChart(c *chartCanvas, spec *chartSpec) {
	entry := spec.Series[0]
	_, hi := seriesBounds(spec.Series)
	valueLabels := len(spec.Labels) <= 12
	headroom := 1.06
	if valueLabels {
		headroom = 1.16
	}
	plotBottom := chartH - chartXLabelSpace - 6
	xFor, yFor := c.drawAxes(spec, 0, max(hi, 0)*headroom, plotBottom)
	n := len(spec.Labels)
	slot := (chartPlotRight - chartPlotLeft) / float64(n)
	barWidth := slot * 0.62
	clr := chartPalette[0]
	for i, value := range entry.Data {
		top := yFor(value)
		c.fillRect(xFor(i)-barWidth/2, top, barWidth, plotBottom-top, clr)
		if valueLabels {
			label := c.fitLabel(humanNumber(value)+spec.Unit, 14, max(slot*0.92, 30))
			c.text(label, xFor(i), top-14, 14, chartTextValue, textAlignCenter, false)
		}
	}
}

func drawGroupedBarChart(c *chartCanvas, spec *chartSpec) {
	_, hi := seriesBounds(spec.Series)
	valueLabels := len(spec.Labels) <= 10 && len(spec.Series) <= 3
	headroom := 1.06
	if valueLabels {
		headroom = 1.16
	}
	plotBottom := chartH - chartXLabelSpace - 6 - chartLegendSpace
	xFor, yFor := c.drawAxes(spec, 0, max(hi, 0)*headroom, plotBottom)
	n := len(spec.Labels)
	count := len(spec.Series)
	slot := (chartPlotRight - chartPlotLeft) / float64(n)
	barWidth := slot * 0.86 / float64(count)
	for i := range spec.Labels {
		for si, entry := range spec.Series {
			value := entry.Data[i]
			top := yFor(value)
			x := xFor(i) - slot*0.43 + float64(si)*barWidth
			c.fillRect(x, top, barWidth*0.92, plotBottom-top, chartPalette[si%len(chartPalette)])
			if valueLabels {
				label := c.fitLabel(humanNumber(value)+spec.Unit, 13, max(barWidth*0.92, 26))
				c.text(label, x+barWidth*0.46, top-13, 13, chartTextValue, textAlignCenter, false)
			}
		}
	}
	c.drawLegend(spec.Series, chartH-chartLegendSpace+16)
}

func drawDonutChart(c *chartCanvas, spec *chartSpec) {
	data := spec.Series[0].Data
	var total float64
	for _, value := range data {
		total += value
	}
	cx, cy := 268.0, 313.0
	rOut, rIn := 188.0, 112.0
	const wedgeGap = 0.03
	angle := -math.Pi / 2
	labelRadius := (rOut + rIn) / 2
	for i, value := range data {
		sweep := value / total * 2 * math.Pi
		c.donutWedge(cx, cy, rOut, rIn, angle+wedgeGap/2, angle+sweep-wedgeGap/2, chartPalette[i%len(chartPalette)])
		if value/total >= 0.05 {
			mid := angle + sweep/2
			labelX := cx + math.Cos(mid)*labelRadius
			labelY := cy + math.Sin(mid)*labelRadius
			c.text(fmt.Sprintf("%.1f%%", value/total*100), labelX, labelY, 14, chartBackground, textAlignCenter, true)
		}
		angle += sweep
	}
	c.text(humanNumber(total)+spec.Unit, cx, cy-12, 27, chartTextTitle, textAlignCenter, true)
	c.text("总计", cx, cy+26, 12, chartTextSub, textAlignCenter, false)

	const (
		legendChip  = 14.0
		legendLeft  = 548.0
		legendText  = 574.0
		legendRight = chartW - 52.0
		lineHeight  = 38.0
	)
	startY := cy - float64(len(data)-1)*lineHeight/2
	for i, label := range spec.Labels {
		y := startY + float64(i)*lineHeight
		c.fillRect(legendLeft, y-legendChip/2, legendChip, legendChip, chartPalette[i%len(chartPalette)])
		valueText := humanNumber(data[i]) + spec.Unit
		valueWidth := c.textWidth(valueText, 15)
		labelMax := (legendRight - valueWidth - 16) - legendText
		c.text(c.fitLabel(label, 15, labelMax), legendText, y, 15, chartTextAxis, textAlignLeft, false)
		c.text(valueText, legendRight, y, 15, chartTextValue, textAlignRight, false)
	}
}
