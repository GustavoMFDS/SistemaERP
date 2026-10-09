package domain

// StockReportRow contains quantity-only information; product costs,
// prices and fiscal classification must never be included in this report.
type StockReportRow struct {
	SKU        string
	Name       string
	Unit       string
	Quantity   string
	Minimum    string
	Active     bool
	BelowLimit bool
}
