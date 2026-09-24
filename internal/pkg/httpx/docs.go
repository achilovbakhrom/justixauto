package httpx

// Response envelopes as generic types for the OpenAPI spec only; handlers
// write them with Data and List. Annotate e.g.
//
//	@Success	200	{object}	httpx.DataEnvelope[CompanyView]
//	@Success	200	{object}	httpx.ListEnvelope[BranchView]

// DataEnvelope documents the body written by Data.
type DataEnvelope[T any] struct {
	Data     T      `json:"data"`
	Revision string `json:"revision"`
	// AsOf is set on GET responses.
	AsOf string `json:"asOf,omitempty"`
}

// ListEnvelope documents the body written by List.
type ListEnvelope[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"nextCursor" extensions:"x-nullable"`
	AsOf       string  `json:"asOf"`
}
