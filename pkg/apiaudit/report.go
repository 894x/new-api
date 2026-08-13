package apiaudit

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

var signedURLPattern = regexp.MustCompile(`https?://[^\s"<>?]+\?[^\s"<>]+`)
var bearerSecretPattern = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/-]+`)
var keySecretPattern = regexp.MustCompile(`(?i)\bsk-[A-Za-z0-9_-]{8,}`)

func BuildReport(config RunConfig, results []CaseResult) Report {
	report := Report{
		Title: "API 准入审计报告", GeneratedAt: time.Now(), Suite: config.Suite,
		BaseURL: redactURL(config.BaseURL), Model: config.Model, Results: append([]CaseResult(nil), results...), APIKey: config.APIKey,
	}
	dimensions := make(map[string]SummaryCounts)
	criticalFailures := make([]string, 0)
	for _, result := range results {
		counts := dimensions[result.Dimension]
		switch result.Status {
		case StatusPass:
			report.Summary.Pass++
			counts.Pass++
		case StatusWarning:
			report.Summary.Warning++
			counts.Warning++
			report.Warnings = append(report.Warnings, result)
		case StatusFail:
			report.Summary.Fail++
			counts.Fail++
			report.Failures = append(report.Failures, result)
			if result.Severity == "critical" {
				criticalFailures = append(criticalFailures, result.ID)
			}
		default:
			report.Summary.Unknown++
			counts.Unknown++
		}
		dimensions[result.Dimension] = counts
	}
	keys := make([]string, 0, len(dimensions))
	for dimension := range dimensions {
		keys = append(keys, dimension)
	}
	sort.Strings(keys)
	for _, dimension := range keys {
		report.Dimensions = append(report.Dimensions, DimensionSummary{Dimension: dimension, SummaryCounts: dimensions[dimension]})
	}
	if len(criticalFailures) > 0 {
		report.Overall = "rejected"
		report.Verdict = "不合格：命中 CRITICAL 项 " + strings.Join(criticalFailures, ", ")
	} else if report.Summary.Fail > 0 {
		report.Overall = "rejected"
		report.Verdict = fmt.Sprintf("不合格：%d 项测试失败", report.Summary.Fail)
	} else if report.Summary.Warning > 0 || report.Summary.Unknown > 0 {
		report.Overall = "review"
		report.Verdict = fmt.Sprintf("需复核：%d 项警告，%d 项无法判定", report.Summary.Warning, report.Summary.Unknown)
	} else {
		report.Overall = "qualified"
		report.Verdict = "合格：所有已执行测试通过"
	}
	return report
}

func WriteReport(outputDir string, report Report) error {
	if outputDir == "" {
		return fmt.Errorf("output directory is required")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	safeReport := report
	safeReport.Results = redactCaseResults(report.Results, report.APIKey)
	safeReport.Failures = redactCaseResults(report.Failures, report.APIKey)
	safeReport.Warnings = redactCaseResults(report.Warnings, report.APIKey)
	rawRoot := filepath.Join(outputDir, "raw")
	for i := range safeReport.Results {
		if !safeResultIDPattern.MatchString(safeReport.Results[i].ID) {
			return fmt.Errorf("unsafe result id %q", safeReport.Results[i].ID)
		}
		artifactDir := filepath.Join(rawRoot, safeReport.Results[i].ID)
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			return fmt.Errorf("create artifact directory: %w", err)
		}
		for j := range safeReport.Results[i].Exchanges {
			exchange := safeReport.Results[i].Exchanges[j]
			encoded, err := common.Marshal(exchange)
			if err != nil {
				return fmt.Errorf("encode exchange: %w", err)
			}
			if err := os.WriteFile(filepath.Join(artifactDir, fmt.Sprintf("exchange-%02d.json", j+1)), encoded, 0o644); err != nil {
				return fmt.Errorf("write exchange: %w", err)
			}
		}
		safeReport.Results[i].ArtifactDir = filepath.ToSlash(filepath.Join("raw", safeReport.Results[i].ID))
	}
	encoded, err := common.Marshal(safeReport)
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "report.json"), encoded, 0o644); err != nil {
		return fmt.Errorf("write JSON report: %w", err)
	}

	tmpl, err := template.New("report").Parse(reportHTMLTemplate)
	if err != nil {
		return fmt.Errorf("parse report template: %w", err)
	}
	file, err := os.Create(filepath.Join(outputDir, "report.html"))
	if err != nil {
		return fmt.Errorf("create HTML report: %w", err)
	}
	executeErr := tmpl.Execute(file, safeReport)
	closeErr := file.Close()
	if executeErr != nil {
		return fmt.Errorf("render HTML report: %w", executeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close HTML report: %w", closeErr)
	}
	return nil
}

func redactCaseResults(results []CaseResult, apiKey string) []CaseResult {
	if len(results) == 0 {
		return nil
	}
	redacted := make([]CaseResult, len(results))
	for i, result := range results {
		result.Evidence = redactText(result.Evidence, apiKey)
		result.Exchanges = append([]HTTPExchange(nil), result.Exchanges...)
		for j := range result.Exchanges {
			result.Exchanges[j].URL = redactURL(result.Exchanges[j].URL)
			result.Exchanges[j].ResponseBody = redactResponseBody(result.Exchanges[j].ResponseBody, apiKey)
			if result.Exchanges[j].RequestBody != nil {
				result.Exchanges[j].RequestBody = redactJSONValue(result.Exchanges[j].RequestBody, apiKey).(map[string]any)
			}
		}
		if result.Usage != nil {
			result.Usage = redactJSONValue(result.Usage, apiKey).(map[string]any)
		}
		redacted[i] = result
	}
	return redacted
}

func redactResponseBody(body, apiKey string) string {
	var value any
	if err := common.Unmarshal([]byte(body), &value); err == nil {
		if encoded, marshalErr := common.Marshal(redactJSONValue(value, apiKey)); marshalErr == nil {
			return string(encoded)
		}
	}
	return redactText(body, apiKey)
}

func redactText(value, apiKey string) string {
	value = signedURLPattern.ReplaceAllStringFunc(value, redactURL)
	value = bearerSecretPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = keySecretPattern.ReplaceAllString(value, "sk-[REDACTED]")
	if apiKey != "" {
		value = strings.ReplaceAll(value, apiKey, "[REDACTED]")
	}
	return value
}

func redactJSONValue(value any, apiKey string) any {
	switch typed := value.(type) {
	case string:
		return redactText(typed, apiKey)
	case []any:
		redacted := make([]any, len(typed))
		for i, item := range typed {
			redacted[i] = redactJSONValue(item, apiKey)
		}
		return redacted
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, item := range typed {
			lowerKey := strings.ToLower(key)
			if lowerKey == "authorization" || lowerKey == "api_key" || lowerKey == "apikey" || lowerKey == "access_token" || lowerKey == "secret" || lowerKey == "token" {
				redacted[key] = "[REDACTED]"
				continue
			}
			redacted[key] = redactJSONValue(item, apiKey)
		}
		return redacted
	default:
		return value
	}
}

const reportHTMLTemplate = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}} - {{.Model}}</title><style>
body{font-family:Inter,"Segoe UI",sans-serif;margin:0;background:#f5f7fb;color:#18212f}.wrap{max-width:1180px;margin:36px auto;padding:0 20px}.hero,.card{background:#fff;border:1px solid #e5e9f0;border-radius:14px;padding:22px;margin-bottom:18px}.hero h1{margin:0 0 8px}.meta{color:#687385}.verdict{font-size:18px;font-weight:700;margin-top:14px}.summary{display:grid;grid-template-columns:repeat(4,1fr);gap:12px}.metric{padding:16px;border-radius:10px;background:#f7f9fc}.metric strong{display:block;font-size:28px}.pass{color:#16803a}.warning{color:#a15c00}.fail{color:#c62828}.unknown{color:#596579}table{width:100%;border-collapse:collapse}th,td{text-align:left;padding:11px;border-bottom:1px solid #edf0f5;vertical-align:top}th{background:#f7f9fc}.tag{font-weight:700;text-transform:uppercase}.evidence{max-width:560px;white-space:pre-wrap;word-break:break-word}@media(max-width:700px){.summary{grid-template-columns:repeat(2,1fr)}table{font-size:13px}}
</style></head><body><main class="wrap">
<section class="hero"><h1>{{.Title}}</h1><div class="meta">地址：{{.BaseURL}}　模型：{{.Model}}　套件：{{.Suite}}　生成时间：{{.GeneratedAt}}</div><div class="verdict {{.Overall}}">{{.Verdict}}</div></section>
<section class="card"><h2>测试总结</h2><div class="summary"><div class="metric pass"><strong>{{.Summary.Pass}}</strong>通过</div><div class="metric warning"><strong>{{.Summary.Warning}}</strong>警告</div><div class="metric fail"><strong>{{.Summary.Fail}}</strong>失败</div><div class="metric unknown"><strong>{{.Summary.Unknown}}</strong>无法判定</div></div></section>
<section class="card"><h2>维度统计</h2><table><thead><tr><th>维度</th><th>通过</th><th>警告</th><th>失败</th><th>无法判定</th></tr></thead><tbody>{{range .Dimensions}}<tr><td>{{.Dimension}}</td><td>{{.Pass}}</td><td>{{.Warning}}</td><td>{{.Fail}}</td><td>{{.Unknown}}</td></tr>{{end}}</tbody></table></section>
{{if .Failures}}<section class="card"><h2>失败项</h2>{{range .Failures}}<p><strong>{{.ID}} {{.Name}}{{if eq .Severity "critical"}} CRITICAL{{end}}</strong>：{{.Evidence}}</p>{{end}}</section>{{end}}
{{if .Warnings}}<section class="card"><h2>警告项</h2>{{range .Warnings}}<p><strong>{{.ID}} {{.Name}}</strong>：{{.Evidence}}</p>{{end}}</section>{{end}}
<section class="card"><h2>测试明细</h2><table><thead><tr><th>测试</th><th>名称</th><th>模型</th><th>维度</th><th>判定</th><th>HTTP</th><th>耗时</th><th>usage</th><th>依据</th><th>原始记录</th></tr></thead><tbody>{{range .Results}}<tr><td>{{.ID}}</td><td>{{.Name}}</td><td>{{.Model}}</td><td>{{.Dimension}}</td><td><span class="tag {{.Status}}">{{.Status}}</span></td><td>{{.HTTPStatus}}</td><td>{{.ElapsedMS}}ms</td><td class="evidence">{{.Usage}}</td><td class="evidence">{{.Evidence}}</td><td>{{if .ArtifactDir}}<a href="{{.ArtifactDir}}/">{{.ArtifactDir}}</a>{{end}}</td></tr>{{end}}</tbody></table></section>
</main></body></html>`
