package domain

// ProductRankingRow shows recorded finalized sale-item totals. It is not
// accounting profit, cash received, or a refund-adjusted revenue metric.
type ProductRankingRow struct {
	ProductID   string `json:"product_id"`
	SKU         string `json:"sku"`
	Name        string `json:"name"`
	Quantity    string `json:"quantity"`
	ItemTotal   string `json:"item_total"`
	SalesCount  int    `json:"sales_count"`
}
