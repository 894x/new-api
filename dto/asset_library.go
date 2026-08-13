package dto

type AssetLibraryFilter struct {
	GroupIds  []string `json:"GroupIds,omitempty"`
	GroupType string   `json:"GroupType,omitempty"`
	Statuses  []string `json:"Statuses,omitempty"`
	Name      string   `json:"Name,omitempty"`
	AssetType string   `json:"AssetType,omitempty"`
}

type CreateAssetGroupRequest struct {
	Name        string  `json:"Name"`
	Description *string `json:"Description,omitempty"`
	GroupType   *string `json:"GroupType,omitempty"`
	ProjectName *string `json:"ProjectName,omitempty"`
}

type CreateAssetRequest struct {
	GroupId     string  `json:"GroupId"`
	URL         string  `json:"URL"`
	AssetType   string  `json:"AssetType"`
	Name        *string `json:"Name,omitempty"`
	ProjectName *string `json:"ProjectName,omitempty"`
}

type ListAssetGroupsRequest struct {
	Filter      *AssetLibraryFilter `json:"Filter,omitempty"`
	PageNumber  *int64              `json:"PageNumber,omitempty"`
	PageSize    *int64              `json:"PageSize,omitempty"`
	SortBy      *string             `json:"SortBy,omitempty"`
	SortOrder   *string             `json:"SortOrder,omitempty"`
	ProjectName *string             `json:"ProjectName,omitempty"`
}

type ListAssetsRequest struct {
	Filter      *AssetLibraryFilter `json:"Filter,omitempty"`
	PageNumber  *int64              `json:"PageNumber,omitempty"`
	PageSize    *int64              `json:"PageSize,omitempty"`
	SortBy      *string             `json:"SortBy,omitempty"`
	SortOrder   *string             `json:"SortOrder,omitempty"`
	ProjectName *string             `json:"ProjectName,omitempty"`
}

type GetAssetGroupRequest struct {
	Id          string  `json:"Id"`
	ProjectName *string `json:"ProjectName,omitempty"`
}

type GetAssetRequest struct {
	Id          string  `json:"Id"`
	ProjectName *string `json:"ProjectName,omitempty"`
}

type UpdateAssetGroupRequest struct {
	Id          string  `json:"Id"`
	Name        *string `json:"Name,omitempty"`
	Description *string `json:"Description,omitempty"`
	ProjectName *string `json:"ProjectName,omitempty"`
}

type UpdateAssetRequest struct {
	Id          string  `json:"Id"`
	Name        *string `json:"Name,omitempty"`
	ProjectName *string `json:"ProjectName,omitempty"`
}

type DeleteAssetRequest struct {
	Id          string  `json:"Id"`
	ProjectName *string `json:"ProjectName,omitempty"`
}

type DeleteAssetGroupRequest struct {
	Id          string  `json:"Id"`
	ProjectName *string `json:"ProjectName,omitempty"`
}

type AssetLibraryError struct {
	Code    string `json:"Code,omitempty"`
	Message string `json:"Message,omitempty"`
}

type AssetReplicaSummary struct {
	Status     string `json:"Status"`
	Ready      int    `json:"Ready"`
	Processing int    `json:"Processing"`
	Failed     int    `json:"Failed"`
	Total      int    `json:"Total"`
}

type AssetGroupResult struct {
	Id          string               `json:"Id"`
	Name        string               `json:"Name"`
	Description string               `json:"Description,omitempty"`
	GroupType   string               `json:"GroupType"`
	ProjectName string               `json:"ProjectName"`
	CreateTime  string               `json:"CreateTime"`
	UpdateTime  string               `json:"UpdateTime"`
	Replication *AssetReplicaSummary `json:"Replication,omitempty"`
}

type AssetResult struct {
	Id                string               `json:"Id"`
	Name              string               `json:"Name,omitempty"`
	URL               string               `json:"URL,omitempty"`
	GroupId           string               `json:"GroupId"`
	AssetType         string               `json:"AssetType"`
	Status            string               `json:"Status,omitempty"`
	Error             *AssetLibraryError   `json:"Error,omitempty"`
	ProjectName       string               `json:"ProjectName"`
	CreateTime        string               `json:"CreateTime"`
	UpdateTime        string               `json:"UpdateTime"`
	LastInferenceTime string               `json:"LastInferenceTime,omitempty"`
	Replication       *AssetReplicaSummary `json:"Replication,omitempty"`
}

type ListAssetGroupsResult struct {
	TotalCount int64              `json:"TotalCount"`
	Items      []AssetGroupResult `json:"Items"`
	PageNumber int64              `json:"PageNumber"`
	PageSize   int64              `json:"PageSize"`
}

type ListAssetsResult struct {
	TotalCount int64         `json:"TotalCount"`
	Items      []AssetResult `json:"Items"`
	PageNumber int64         `json:"PageNumber"`
	PageSize   int64         `json:"PageSize"`
}
