package seedance_sls

import "strings"

// Keep Seedance SLS model support and pricing self-contained. Do not depend on
// the Doubao task adaptor: the two channels have independent lifecycles.
var modelList = []string{
	"doubao-seedance-1-0-pro-250528",
	"doubao-seedance-1-0-lite-t2v",
	"doubao-seedance-1-0-lite-i2v",
	"doubao-seedance-1-5-pro-251215",
	"doubao-seedance-2-0-260128",
	"doubao-seedance-2-0-fast-260128",
	"doubao-seedance-2-0-mini-260615",
	"doubao-seedance-2-5-260628",
}

type videoPriceKey struct {
	is1080p  bool
	is4k     bool
	hasVideo bool
}

// videoPriceTable stores the Seedance SLS price per million tokens for each
// output-resolution and video-input combination. The zero-value key is the
// 480p/720p text-to-video base price configured through ModelRatio.
var videoPriceTable = map[string]map[videoPriceKey]float64{
	"doubao-seedance-2-0-260128": {
		{hasVideo: false}:                46.0,
		{hasVideo: true}:                 28.0,
		{is1080p: true, hasVideo: false}: 51.0,
		{is1080p: true, hasVideo: true}:  31.0,
		{is4k: true, hasVideo: false}:    26.0,
		{is4k: true, hasVideo: true}:     16.0,
	},
	"doubao-seedance-2-0-fast-260128": {
		{hasVideo: false}: 37.0,
		{hasVideo: true}:  22.0,
	},
	"doubao-seedance-2-0-mini-260615": {
		{hasVideo: false}: 23.0,
		{hasVideo: true}:  14.0,
	},
	"doubao-seedance-2-5-260628": {
		{hasVideo: false}:                70.0,
		{hasVideo: true}:                 42.0,
		{is1080p: true, hasVideo: false}: 77.0,
		{is1080p: true, hasVideo: true}:  46.0,
	},
}

func getVideoInputRatio(modelName, resolution string, hasVideo bool) (float64, bool) {
	prices, ok := videoPriceTable[modelName]
	base := prices[videoPriceKey{}]
	if !ok || base <= 0 {
		return 0, false
	}
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	price, ok := prices[videoPriceKey{
		is1080p:  resolution == "1080p",
		is4k:     resolution == "4k",
		hasVideo: hasVideo,
	}]
	if !ok {
		return 1.0, true
	}
	return price / base, true
}
