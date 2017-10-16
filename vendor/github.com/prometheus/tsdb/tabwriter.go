package tsdb

import (
	"io"
	"text/tabwriter"
)

const (
	minwidth = 0
	tabwidth = 0
	padding  = 2
	padchar  = ' '
	flags    = 0
)

func GetNewTabWriter(output io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(output, minwidth, tabwidth, padding, padchar, flags)
}
