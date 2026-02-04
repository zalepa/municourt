package parser

// MunicipalityStats holds all statistics for a single municipality page.
type MunicipalityStats struct {
	County        string             `json:"county"`
	Municipality  string             `json:"municipality"`
	DateRange     string             `json:"dateRange"`
	Filings       SectionWithChange  `json:"filings"`
	Resolutions   SectionWithChange  `json:"resolutions"`
	Clearance     SectionTwoRow      `json:"clearance"`
	ClearancePct  SectionTwoRow      `json:"clearancePercent"`
	Backlog       SectionWithChange  `json:"backlog"`
	BacklogPer100 SectionWithChange  `json:"backlogPer100MthlyFilings"`
	BacklogPct    SectionTwoRow      `json:"backlogPercent"`
	ActivePending SectionWithChange  `json:"activePending"`
}

// SectionWithChange has three sub-rows: prior period, current period, and % change.
type SectionWithChange struct {
	PriorPeriod   RowData `json:"priorPeriod"`
	CurrentPeriod RowData `json:"currentPeriod"`
	PctChange     RowData `json:"pctChange"`
}

// SectionTwoRow has two sub-rows: prior period and current period.
type SectionTwoRow struct {
	PriorPeriod   RowData `json:"priorPeriod"`
	CurrentPeriod RowData `json:"currentPeriod"`
}

// RowData holds one row of column values. Values are strings because they may
// contain "%", "- -", commas, or negative signs.
type RowData struct {
	Label         string `json:"label"`
	Indictables   string `json:"indictables"`
	DPAndPDP      string `json:"dpAndPdp"`
	OtherCriminal string `json:"otherCriminal"`
	CriminalTotal string `json:"criminalTotal"`
	DWI           string `json:"dwi"`
	TrafficMoving string `json:"trafficMoving"`
	Parking       string `json:"parking"`
	TrafficTotal  string `json:"trafficTotal"`
	GrandTotal    string `json:"grandTotal"`
}
