package source

import "github.com/mshaulsky/domovoi/internal/model"

// Batch is what one pass of a source produced.
type Batch struct {
	Devices  []model.Device // nil = unchanged since the previous batch
	Readings []model.Reading
	Events   []model.Event // observed by the source itself, beside those the state derives from changes
}
