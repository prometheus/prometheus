package main

import (
	"bytes"
	"fmt"
	"os"
)

func DebugMetrics() int {
	tw := NewTarGzFileFileWriter(TarGzFileWriterConfig{FileName: "debug.tar.gz"})
	defer func() {
		if err := tw.Close(); err != nil {
			panic(err)
		}
	}()
	body, err := NewBodyGetter(BodyGetterConfig{Path: "/metrics"}).Get()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	var buf bytes.Buffer
	buf.Write(body)
	if err := tw.AddFile("metrics.txt", buf); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	fmt.Println(buf.String())
	return 0
}
