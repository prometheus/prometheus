// Package location provides the Location aggregate.
package location

import (
	"errors"
)

// UNLocode is the United Nations location code that uniquely identifies a
// particular location.
//
// http://www.unece.org/cefact/locode/
// http://www.unece.org/cefact/locode/DocColumnDescription.htm#LOCODE
type UNLocode string

// Location is a location is our model is stops on a journey, such as cargo
// origin or destination, or carrier movement endpoints.
type Location struct {
	UNLocode UNLocode
	Name     string
}

// ErrUnknown is used when a location could not be found.
var ErrUnknown = errors.New("unknown location")

// Repository provides access a location store.
type Repository interface {
	Find(locode UNLocode) (*Location, error)
	FindAll() []*Location
}
