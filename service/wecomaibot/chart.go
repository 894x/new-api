package wecomaibot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// 企微单帧最多 10 张图、单图 base64 前最大 10MB；这里收紧为 3 张 / 4MB，
// 留足协议余量并控制流量与渲染耗时。
const (
	maxChartImages  = 3
	maxChartPNGByte = 4 << 20
)

// chartSpec 是渲染引擎消费的图表规格；约束与 buildChartSpec 的校验一致。
type chartSpec struct {
	Kind     string        `json:"kind"`
	Title    string        `json:"title"`
	Subtitle string        `json:"subtitle,omitempty"`
	YLabel   string        `json:"y_label,omitempty"`
	Unit     string        `json:"unit,omitempty"`
	Labels   []string      `json:"labels"`
	Series   []chartSeries `json:"series"`
}

type chartSeries struct {
	Name string    `json:"name"`
	Data []float64 `json:"data"`
}

func (s *chartSpec) validate() error {
	switch s.Kind {
	case "line", "bar", "grouped_bar":
	case "donut":
		var total float64
		for _, value := range s.Series[0].Data {
			total += value
		}
		if total <= 0 {
			return errors.New("donut 数据总和必须大于 0")
		}
	default:
		return errors.New("kind 必须是 line、bar、grouped_bar 或 donut")
	}
	return nil
}

// chartCollector 收集一次对话里工具生成的图表，随最终流式帧（finish=true
// 的 msg_item）一并下发。图片不过模型，直接附在回复末尾。
type chartCollector struct {
	mu     sync.Mutex
	images [][]byte
}

// add 收录一张图表，返回它在回复中的序号（1 起）；超限返回 0。
func (c *chartCollector) add(png []byte) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.images) >= maxChartImages {
		return 0
	}
	c.images = append(c.images, png)
	return len(c.images)
}

func (c *chartCollector) drain() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	images := c.images
	c.images = nil
	return images
}

type chartCollectorKeyType struct{}

var chartCollectorKey chartCollectorKeyType

func chartCollectorFromContext(ctx context.Context) (*chartCollector, bool) {
	collector, ok := ctx.Value(chartCollectorKey).(*chartCollector)
	return collector, ok
}

// renderChartToolHandler 是暴露给模型的 render_chart 工具：校验规格并渲染，
// 图片进收集器，给模型回执确认。
func renderChartToolHandler(ctx context.Context, args map[string]any) (any, error) {
	spec, err := buildChartSpec(args)
	if err != nil {
		return nil, err
	}
	collector, ok := chartCollectorFromContext(ctx)
	if !ok {
		return nil, errors.New("chart output is unavailable in this context")
	}
	png, err := renderChartPNG(spec)
	if err != nil {
		common.SysLog("wecom ai bot chart render failed: " + err.Error())
		return nil, fmt.Errorf("图表渲染失败: %w", err)
	}
	index := collector.add(png)
	if index == 0 {
		return map[string]any{
			"rendered": false,
			"error":    fmt.Sprintf("单次回复最多附 %d 张图表，请直接用文字总结", maxChartImages),
		}, nil
	}
	return map[string]any{
		"rendered":    true,
		"chart_index": index,
		"note":        "图表已生成，会作为图片消息跟在文字回复之后；文字部分请做要点总结，不要重复图表中的全部数字",
	}, nil
}

// buildChartSpec 从模型参数组装并校验图表规格。
func buildChartSpec(args map[string]any) (*chartSpec, error) {
	kind := argString(args, "kind")
	if kind != "line" && kind != "bar" && kind != "grouped_bar" && kind != "donut" {
		return nil, errors.New("kind 必须是 line、bar、grouped_bar 或 donut")
	}
	title := strings.TrimSpace(argString(args, "title"))
	if title == "" {
		return nil, errors.New("title 不能为空")
	}
	if len([]rune(title)) > 80 {
		return nil, errors.New("title 过长（最多 80 字符）")
	}

	rawLabels, _ := args["labels"].([]any)
	if len(rawLabels) < 1 || len(rawLabels) > 60 {
		return nil, errors.New("labels 必须是 1-60 个字符串")
	}
	labels := make([]string, 0, len(rawLabels))
	for _, item := range rawLabels {
		label, ok := item.(string)
		if !ok {
			return nil, errors.New("labels 必须全部是字符串")
		}
		labels = append(labels, truncateRunes(label, 24))
	}

	rawSeries, _ := args["series"].([]any)
	if len(rawSeries) < 1 || len(rawSeries) > 6 {
		return nil, errors.New("series 必须是 1-6 组数据")
	}
	if kind == "donut" && len(rawSeries) != 1 {
		return nil, errors.New("donut 只允许一组数据")
	}
	series := make([]chartSeries, 0, len(rawSeries))
	for index, item := range rawSeries {
		entry, _ := item.(map[string]any)
		if entry == nil {
			return nil, errors.New("series 元素必须是对象")
		}
		name := truncateRunes(argString(entry, "name"), 60)
		rawData, _ := entry["data"].([]any)
		if len(rawData) != len(labels) {
			return nil, errors.New("series 数据长度必须与 labels 一致")
		}
		data := make([]float64, 0, len(rawData))
		for _, value := range rawData {
			number, ok := toFloat(value)
			if !ok {
				return nil, errors.New("series.data 必须全部是数字")
			}
			data = append(data, number)
		}
		if kind == "donut" && index == 0 {
			var total float64
			for _, value := range data {
				total += value
			}
			if total <= 0 {
				return nil, errors.New("donut 数据总和必须大于 0")
			}
		}
		series = append(series, chartSeries{Name: name, Data: data})
	}

	spec := &chartSpec{
		Kind:   kind,
		Title:  title,
		Labels: labels,
		Series: series,
	}
	if subtitle := strings.TrimSpace(argString(args, "subtitle")); subtitle != "" {
		spec.Subtitle = truncateRunes(subtitle, 100)
	}
	if yLabel := strings.TrimSpace(argString(args, "y_label")); yLabel != "" {
		spec.YLabel = truncateRunes(yLabel, 40)
	}
	if unit := strings.TrimSpace(argString(args, "unit")); unit != "" {
		spec.Unit = truncateRunes(unit, 12)
	}
	return spec, nil
}

// toFloat 统一把 JSON 数字（float64）或整数转为 float64。
func toFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}
