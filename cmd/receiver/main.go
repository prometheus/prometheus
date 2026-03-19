package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func receiveHandler(w http.ResponseWriter, r *http.Request) {
	compressed, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req prompb.WriteRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	for _, ts := range req.Timeseries {
		labels := "{"
		for i, l := range ts.Labels {
			if i > 0 {
				labels += ", "
			}
			labels += fmt.Sprintf("%s=%q", l.Name, l.Value)
		}
		labels += "}"
		fmt.Printf("Received Series: %s with %d samples\n", labels, len(ts.Samples))
	}
}

func main() {
	http.HandleFunc("/api/v1/write", receiveHandler)
	log.Println("Listening on :9091...")
	log.Fatal(http.ListenAndServe(":9091", nil))
}
