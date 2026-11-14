package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/shopspring/decimal"
	"github.com/tidwall/gjson"
)

// Gross-profit reporting observes settled charges; it never changes billing.
// The snapshot lives in root_info so ordinary administrators cannot read costs.
const grossProfitOutputVATRate = 0.06

type consumeGrossProfit struct {
	EventID          string   `json:"event_id,omitempty"`
	Version          int      `json:"version"`
	ChannelDiscount  *float64 `json:"channel_discount,omitempty"`
	CustomerDiscount string   `json:"customer_discount,omitempty"`
	CostUSD          *string  `json:"cost_usd,omitempty"`
	OutputVATRate    *float64 `json:"output_vat_rate,omitempty"`
	InputVATRate     *float64 `json:"input_vat_rate,omitempty"`
	Reason           string   `json:"reason,omitempty"`
}

// Rates are fractions of the tax-exclusive amount. Nil means unknown; zero
// is an explicitly configured zero-rate invoice. Input invoices are deductible.
func ValidateChannelInvoiceVATRate(rate *float64) error {
	if rate != nil && (math.IsNaN(*rate) || math.IsInf(*rate, 0) || *rate < 0 || *rate > 1) {
		return errors.New("channel invoice VAT rate must be between 0 and 1")
	}
	return nil
}

func recordConsumeGrossProfit(params RecordConsumeLogParams) {
	snapshot := consumeGrossProfit{Version: 2, Reason: "discount_unconfigured", EventID: common.NewRequestId(), OutputVATRate: common.GetPointer(grossProfitOutputVATRate)}
	defer func() { params.Other.SetRoot("gross_profit", snapshot) }()
	var channel Channel
	if err := DB.Select("id", "models", "cost_discount", "invoice_vat_rate").First(&channel, params.ChannelId).Error; err != nil {
		snapshot.Reason = "discount_unavailable"
		return
	}
	snapshot.InputVATRate = channel.InvoiceVATRate
	modelName := params.ModelName
	if !slices.Contains(strings.Split(channel.Models, ","), modelName) {
		modelName = ratio_setting.RoutingMatchModelName(modelName)
	}
	overrides, err := getChannelModelOverrideMap(DB, channel.Id)
	if err != nil {
		snapshot.Reason = "discount_unavailable"
		return
	}
	var override *ChannelModelOverride
	if value, ok := overrides[modelName]; ok {
		override = &value
	}
	snapshot.ChannelDiscount = effectiveChannelModelRouting(&channel, modelName, override).EffectiveCostDiscount
	if snapshot.ChannelDiscount == nil {
		return
	}
	if ValidateChannelCostDiscount(snapshot.ChannelDiscount) != nil || params.Quota < 0 {
		snapshot.Reason = "billing_metadata_missing"
		return
	}
	// An explicit zero cost is known even if the customer received a free call.
	if *snapshot.ChannelDiscount == 0 {
		zero := "0"
		snapshot.CostUSD, snapshot.Reason = &zero, ""
		return
	}
	metadata := gjson.Parse(common.GetJsonString(params.Other.public))
	quotaPerUnit, err := decimal.NewFromString(metadata.Get("quota_per_unit").Raw)
	if err != nil || !quotaPerUnit.IsPositive() {
		snapshot.Reason = "billing_metadata_missing"
		return
	}
	var original decimal.Decimal
	if raw := metadata.Get("group_model_discount.original_quota"); raw.Exists() {
		// Monthly tiers can cross several discounts, including zero. Settlement
		// already records their exact before-discount amount.
		original, err = decimal.NewFromString(raw.Raw)
	} else {
		var discount decimal.Decimal
		discount, err = decimal.NewFromString(metadata.Get("group_ratio").Raw)
		if err == nil && discount.IsPositive() {
			original = decimal.NewFromInt(int64(params.Quota)).Div(discount)
		} else {
			snapshot.Reason = "billing_metadata_missing"
			return
		}
	}
	if err != nil || original.IsNegative() {
		snapshot.Reason = "billing_metadata_missing"
		return
	}
	if original.IsPositive() {
		snapshot.CustomerDiscount = decimal.NewFromInt(int64(params.Quota)).Div(original).String()
	}
	cost := original.Mul(decimal.NewFromFloat(*snapshot.ChannelDiscount)).Div(quotaPerUnit)
	value, _ := cost.Float64()
	if math.IsInf(value, 0) || math.IsNaN(value) {
		snapshot.Reason = "billing_metadata_missing"
		return
	}
	costString := cost.String()
	snapshot.CostUSD, snapshot.Reason = &costString, ""
}

type GrossProfitTotals struct {
	KnownRequests     int64    `json:"known_requests"`
	KnownRevenueUSD   float64  `json:"known_revenue_usd"`
	CostUSD           float64  `json:"cost_usd"`
	NetRevenueUSD     float64  `json:"net_revenue_usd"`
	NetCostUSD        float64  `json:"net_cost_usd"`
	OutputVATUSD      float64  `json:"output_vat_usd"`
	InputVATUSD       float64  `json:"input_vat_usd"`
	ProfitUSD         float64  `json:"profit_usd"`
	ProfitMargin      *float64 `json:"profit_margin"`
	RevenueCoverage   *float64 `json:"revenue_coverage"`
	UnknownRequests   int64    `json:"unknown_requests"`
	UnknownRevenueUSD float64  `json:"unknown_revenue_usd"`
}

type GrossProfitChannel struct {
	ChannelID   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	GrossProfitTotals
}

type GrossProfitUnknown struct {
	ChannelID   int     `json:"channel_id"`
	ChannelName string  `json:"channel_name"`
	Reason      string  `json:"reason"`
	Requests    int64   `json:"requests"`
	RevenueUSD  float64 `json:"revenue_usd"`
}

type GrossProfitPoint struct {
	Timestamp int64  `json:"timestamp"`
	Day       string `json:"day"`
	GrossProfitTotals
}

type GrossProfitReport struct {
	Timezone    string                 `json:"timezone"`
	Aggregation GrossProfitAggregation `json:"aggregation"`
	Summary     GrossProfitTotals      `json:"summary"`
	Channels    []GrossProfitChannel   `json:"channels"`
	Unknown     []GrossProfitUnknown   `json:"unknown"`
	Series      []GrossProfitPoint     `json:"series"`
}

type GrossProfitAggregation struct {
	BackfillComplete  bool  `json:"backfill_complete"`
	UnassignedHistory bool  `json:"unassigned_history"`
	PendingEvents     int64 `json:"pending_events"`
	UpdatedAt         int64 `json:"updated_at"`
}

type grossProfitAccumulator struct {
	knownRequests, unknownRequests int64
	revenue, cost, unknownRevenue  decimal.Decimal
	netRevenue, netCost            decimal.Decimal
}

func (a *grossProfitAccumulator) add(values grossProfitAccumulator) {
	a.knownRequests += values.knownRequests
	a.unknownRequests += values.unknownRequests
	a.revenue = a.revenue.Add(values.revenue)
	a.cost = a.cost.Add(values.cost)
	a.netRevenue = a.netRevenue.Add(values.netRevenue)
	a.netCost = a.netCost.Add(values.netCost)
	a.unknownRevenue = a.unknownRevenue.Add(values.unknownRevenue)
}

func (a *grossProfitAccumulator) totals() GrossProfitTotals {
	result := GrossProfitTotals{KnownRequests: a.knownRequests, UnknownRequests: a.unknownRequests}
	result.KnownRevenueUSD, _ = a.revenue.Float64()
	result.CostUSD, _ = a.cost.Float64()
	result.NetRevenueUSD, _ = a.netRevenue.Float64()
	result.NetCostUSD, _ = a.netCost.Float64()
	result.OutputVATUSD, _ = a.revenue.Sub(a.netRevenue).Float64()
	result.InputVATUSD, _ = a.cost.Sub(a.netCost).Float64()
	result.ProfitUSD, _ = a.netRevenue.Sub(a.netCost).Float64()
	result.UnknownRevenueUSD, _ = a.unknownRevenue.Float64()
	if a.netRevenue.IsPositive() {
		value, _ := a.netRevenue.Sub(a.netCost).Div(a.netRevenue).Float64()
		result.ProfitMargin = &value
	}
	if total := a.revenue.Add(a.unknownRevenue); total.IsPositive() {
		value, _ := a.revenue.Div(total).Float64()
		result.RevenueCoverage = &value
	}
	return result
}

// grossProfitDayStart finds the first instant of a local date, including dates
// whose midnight is skipped or repeated by a timezone transition.
func grossProfitDayStart(day string, location *time.Location) (time.Time, error) {
	date, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return time.Time{}, err
	}
	windowEnd := date.Add(48 * time.Hour)
	for probe := date.Add(-48 * time.Hour); probe.Before(windowEnd); {
		local := probe.In(location)
		_, offset := local.Zone()
		_, zoneEnd := local.ZoneBounds()
		segmentEnd := windowEnd
		if !zoneEnd.IsZero() && zoneEnd.Before(segmentEnd) {
			segmentEnd = zoneEnd
		}
		candidate := date.Add(-time.Duration(offset) * time.Second)
		if candidate.Before(probe) {
			candidate = probe
		}
		if candidate.Before(segmentEnd) && candidate.In(location).Format(time.DateOnly) == day {
			return candidate.In(location), nil
		}
		probe = segmentEnd
	}
	return time.Time{}, errors.New("date does not exist in timezone")
}

func grossProfitBucketStart(timestamp int64) int64 {
	return timestamp - (timestamp%grossProfitBucketSeconds+grossProfitBucketSeconds)%grossProfitBucketSeconds
}

// GetGrossProfitReport groups timezone-neutral rollups by the browser's local
// calendar. endDay is exclusive. Raw usage logs are never queried here.
func GetGrossProfitReport(ctx context.Context, startDay, endDay, timezone string) (*GrossProfitReport, error) {
	if timezone == "" {
		timezone = "UTC"
	}
	if len(timezone) > 64 || timezone == "Local" {
		return nil, errors.New("invalid timezone")
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, errors.New("invalid timezone")
	}
	start, err := grossProfitDayStart(startDay, location)
	if err != nil {
		return nil, errors.New("invalid start_date")
	}
	end, err := grossProfitDayStart(endDay, location)
	if err != nil || !end.After(start) {
		return nil, errors.New("invalid end_date")
	}
	var state GrossProfitRollupState
	if err := DB.WithContext(ctx).First(&state, "name = ?", GrossProfitRollupName).Error; err != nil {
		return nil, err
	}
	report := &GrossProfitReport{Timezone: timezone, Channels: []GrossProfitChannel{}, Unknown: []GrossProfitUnknown{}, Series: []GrossProfitPoint{}}
	report.Aggregation.BackfillComplete = state.BackfillComplete && state.LegacyBackfillComplete && state.VATBackfillComplete
	report.Aggregation.UpdatedAt = state.UpdatedAt
	if err := DB.WithContext(ctx).Model(&GrossProfitEvent{}).Where("applied_at = ? AND source_created_at IS NOT NULL", 0).Count(&report.Aggregation.PendingEvents).Error; err != nil {
		return nil, err
	}
	if report.Aggregation.BackfillComplete {
		var missing []string
		if err := DB.WithContext(ctx).Model(&GrossProfitEvent{}).Where("source_created_at IS NULL").Limit(1).Pluck("event_key", &missing).Error; err != nil {
			return nil, err
		}
		report.Aggregation.UnassignedHistory = len(missing) != 0
	}
	var buckets []GrossProfitBucket
	if err := DB.WithContext(ctx).Where("bucket_ts >= ? AND bucket_ts < ?", grossProfitBucketStart(start.Unix()), end.Unix()).Order("bucket_ts ASC").Find(&buckets).Error; err != nil {
		return nil, err
	}
	// Modern timezone boundaries align with 15-minute UTC buckets. For an
	// historical offset that splits a bucket, use only that bucket's durable
	// timestamped events so no partial day is rounded or counted twice.
	boundaryBuckets := map[int64]struct{}{}
	wholeBuckets := buckets[:0]
	for _, bucket := range buckets {
		bucketEnd := bucket.BucketTs + grossProfitBucketSeconds
		firstDay := time.Unix(bucket.BucketTs, 0).In(location).Format(time.DateOnly)
		lastDay := time.Unix(bucketEnd-1, 0).In(location).Format(time.DateOnly)
		splitsDay := firstDay != lastDay
		_, transition := time.Unix(bucket.BucketTs, 0).In(location).ZoneBounds()
		for !transition.IsZero() && transition.Unix() < bucketEnd {
			if transition.Add(-time.Second).In(location).Format(time.DateOnly) != firstDay || transition.In(location).Format(time.DateOnly) != firstDay {
				splitsDay = true
				break
			}
			_, transition = transition.In(location).ZoneBounds()
		}
		if bucket.BucketTs < start.Unix() || bucketEnd > end.Unix() || splitsDay {
			boundaryBuckets[bucket.BucketTs] = struct{}{}
			continue
		}
		wholeBuckets = append(wholeBuckets, bucket)
	}
	buckets = wholeBuckets
	for bucketTs := range boundaryBuckets {
		var events []GrossProfitEvent
		if err := DB.WithContext(ctx).Where("source_created_at >= ? AND source_created_at < ? AND applied_at <> ?",
			max(bucketTs, start.Unix()), min(bucketTs+grossProfitBucketSeconds, end.Unix()), 0).Find(&events).Error; err != nil {
			return nil, err
		}
		for _, event := range events {
			buckets = append(buckets, GrossProfitBucket{BucketTs: *event.SourceCreatedAt, ChannelID: event.ChannelID, Reason: event.Reason,
				Requests: 1, RevenueUSD: event.RevenueUSD, CostUSD: event.CostUSD, NetRevenueUSD: event.NetRevenueUSD, NetCostUSD: event.NetCostUSD})
		}
	}
	var total grossProfitAccumulator
	channels := map[int]*grossProfitAccumulator{}
	days := map[string]*grossProfitAccumulator{}
	type unknownKey struct {
		channel int
		reason  string
	}
	unknown := map[unknownKey]*grossProfitAccumulator{}
	for _, bucket := range buckets {
		day := time.Unix(bucket.BucketTs, 0).In(location).Format(time.DateOnly)
		revenue, err := decimal.NewFromString(bucket.RevenueUSD)
		if err != nil {
			return nil, err
		}
		values := grossProfitAccumulator{}
		if bucket.Reason == "" {
			values.knownRequests, values.revenue = bucket.Requests, revenue
			for _, amount := range []struct {
				raw    string
				target *decimal.Decimal
			}{{bucket.CostUSD, &values.cost}, {bucket.NetRevenueUSD, &values.netRevenue}, {bucket.NetCostUSD, &values.netCost}} {
				parsed, err := decimal.NewFromString(amount.raw)
				if err != nil {
					return nil, err
				}
				*amount.target = parsed
			}
		} else {
			values.unknownRequests, values.unknownRevenue = bucket.Requests, revenue
		}
		total.add(values)
		if channels[bucket.ChannelID] == nil {
			channels[bucket.ChannelID] = &grossProfitAccumulator{}
		}
		channels[bucket.ChannelID].add(values)
		if days[day] == nil {
			days[day] = &grossProfitAccumulator{}
		}
		days[day].add(values)
		if bucket.Reason != "" {
			key := unknownKey{bucket.ChannelID, bucket.Reason}
			if unknown[key] == nil {
				unknown[key] = &grossProfitAccumulator{}
			}
			unknown[key].add(values)
		}
	}
	ids := make([]int, 0, len(channels))
	for id := range channels {
		ids = append(ids, id)
	}
	names := map[int]string{}
	if len(ids) > 0 {
		var records []Channel
		if err := DB.WithContext(ctx).Select("id", "name").Where("id IN ?", ids).Find(&records).Error; err != nil {
			return nil, err
		}
		for _, channel := range records {
			names[channel.Id] = channel.Name
		}
	}
	for id, values := range channels {
		name := names[id]
		if name == "" {
			name = "#" + strconv.Itoa(id)
		}
		if values.knownRequests > 0 {
			report.Channels = append(report.Channels, GrossProfitChannel{id, name, values.totals()})
		}
	}
	for key, values := range unknown {
		name := names[key.channel]
		if name == "" {
			name = fmt.Sprintf("#%d", key.channel)
		}
		report.Unknown = append(report.Unknown, GrossProfitUnknown{key.channel, name, key.reason, values.unknownRequests, values.totals().UnknownRevenueUSD})
	}
	for day, values := range days {
		date, err := grossProfitDayStart(day, location)
		if err != nil {
			return nil, err
		}
		report.Series = append(report.Series, GrossProfitPoint{Timestamp: date.Unix(), Day: day, GrossProfitTotals: values.totals()})
	}
	slices.SortFunc(report.Series, func(a, b GrossProfitPoint) int { return strings.Compare(a.Day, b.Day) })
	slices.SortFunc(report.Channels, func(a, b GrossProfitChannel) int { return a.ChannelID - b.ChannelID })
	slices.SortFunc(report.Unknown, func(a, b GrossProfitUnknown) int {
		if a.ChannelID != b.ChannelID {
			return a.ChannelID - b.ChannelID
		}
		return strings.Compare(a.Reason, b.Reason)
	})
	report.Summary = total.totals()
	return report, nil
}
