package columnar

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/parquet-go/parquet-go"
)

func buildSchemaForLabels(labels []string) *parquet.Schema {
	node := parquet.Group{}
	for _, label := range labels {
		node["l_"+label] = parquet.String()
	}
	return parquet.NewSchema("metric_family", node)
}

func queryDictionary(fileName, column string) {
	f, err := os.Open(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
}

// query does something like: SELECT project FROM fileName WHERE lname = lvalue
func query(fileName, lname, lvalue string, project ...string) []parquet.Row {
	f, err := os.Open(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	fmt.Printf("\nQuerying: select (%v) where %s = %s\n", project, lname, lvalue)

	// We build a schema only with the columns we are interested in. See NewGenericReader docstring:
	// > If the option list may explicitly declare a schema, it must be compatible
	// > with the schema generated from T.
	// Note that I'm not including the chunks, but that should be done if we want the data.
	schema := buildSchemaForLabels(append(project, lname))
	reader := parquet.NewGenericReader[any](f, schema)
	defer reader.Close()

	var result []parquet.Row
	buf := make([]parquet.Row, 10)

	rowId := 0 // We don't use this, but could be useful if we wanted to reference a row.
	for {
		readRows, err := reader.ReadRows(buf)
		if err != nil && err != io.EOF {
			log.Fatal(err)
		}
		if readRows == 0 {
			break
		}

		for _, row := range buf[:readRows] {
			for _, val := range row {
				colName := reader.Schema().Columns()[val.Column()][0]
				// TODO: if we have dict encoding, is there a way to make this comparison quicker?
				if colName == "l_"+lname && string(val.ByteArray()) == lvalue {
					result = append(result, row)
					break
				}
			}
			rowId++
		}

		if err == io.EOF {
			break
		}

	}

	fmt.Println("\nQuery result:")
	for i, row := range result {
		fmt.Printf("\nRow %d:\n", i+1)
		for j, val := range row {
			fmt.Printf("\t%d: %s\n", j, string(val.ByteArray()))
		}
	}
	return result
}
